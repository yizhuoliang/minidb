package routines

import (
	"log"
	"minidb/msg"
)

type DataManagerState struct {
	managerNumber    int
	isUp             bool
	table            [21][]msg.Pair
	readyTable       [21]bool
	failureRecords   map[uint64]uint64 // map from crashing time to restore time
	accessRecords    map[int]uint64    // map from txnNumber to the earliest time of accessing this DM
	commandChan      chan *msg.Command
	responseChan     chan *msg.Response
	uncommitedWrites []msg.Pair
}

func NewDataManager(managerNumber int, commandChan chan *msg.Command, responseChan chan *msg.Response, initTime uint64) DataManagerState {
	// build a new DM state
	newDM := DataManagerState{
		managerNumber:    managerNumber,
		isUp:             true,
		commandChan:      commandChan,
		responseChan:     responseChan,
		failureRecords:   make(map[uint64]uint64),
		accessRecords:    make(map[int]uint64),
		uncommitedWrites: make([]msg.Pair, 0),
	}

	// initialize the data tables
	for key := 2; key <= 20; key += 2 {
		newDM.table[key] = []msg.Pair{{Key: key, Value: key * 10, Timestamp: initTime}}
		newDM.readyTable[key] = true
	}
	// only even indexed sites have monopolized keys
	if managerNumber%2 == 0 {
		key1 := managerNumber - 1
		key2 := managerNumber + 9
		newDM.table[key1] = []msg.Pair{{Key: key1, Value: key1 * 10, Timestamp: initTime}}
		newDM.table[key2] = []msg.Pair{{Key: key2, Value: key2 * 10, Timestamp: initTime}}
		newDM.readyTable[key1] = true
		newDM.readyTable[key2] = true
	}

	return newDM
}

func (s *DataManagerState) DataManagerRoutine() {
	for {
		command, ok := <-s.commandChan
		if !ok {
			log.Fatal("ERROR: commandChan closed!")
		}

		switch command.Type {
		case msg.Read:
			// if down, always respond error
			if !s.isUp {
				s.responseChan <- &msg.Response{Type: msg.Error}
				continue
			}

			key := command.Pair.Key
			served := false
			if s.readyTable[key] && s.table[key] != nil {
				versions := s.table[key]
				for i := len(versions) - 1; i >= 0; i-- {
					if versions[i].Timestamp == command.VersionNeeded {
						// this request is served, so we should record the access history
						_, ok := s.accessRecords[command.TxnNumber]
						if !ok {
							s.accessRecords[command.TxnNumber] = command.ExtraTimestamp
						}
						s.responseChan <- &msg.Response{Type: msg.Read, ManagerNumber: s.managerNumber, Pair: versions[i]}
						served = true
						break
					}
				}
			}
			if !served {
				s.responseChan <- &msg.Response{Type: msg.Error}
			}

		case msg.Write:
			// just record the time of access
			_, ok := s.accessRecords[command.TxnNumber]
			if !ok {
				s.accessRecords[command.TxnNumber] = command.Timestamp
			}
			s.uncommitedWrites = append(s.uncommitedWrites, command.Pair)
			if s.isUp {
				s.responseChan <- &msg.Response{Type: msg.Success}
			} else {
				s.responseChan <- &msg.Response{Type: msg.Error}
			}

		case msg.Commit:
			if !s.isUp {
				s.responseChan <- &msg.Response{Type: msg.Error}
				continue
			}
			// if this DM ever failed after the txn's first access to this DM, or never accessed this DM
			// then this DM should not take this commit
			earliestAccess, ok := s.accessRecords[command.TxnNumber]
			if !ok || isOverlapping(s.failureRecords, earliestAccess, command.Timestamp) {
				s.responseChan <- &msg.Response{Type: msg.Error}
				continue
			}
			commitedKeys := make([]int, 0)
			for _, write := range *command.WritesToCommit {
				// if the table entry is empty, this dataManager don't maintain this variable
				// so it can ignore this update
				if len(s.table[write.Key]) == 0 {
					continue
				}
				s.table[write.Key] = append(s.table[write.Key], msg.Pair{Key: write.Key, Value: write.Value, Timestamp: command.Timestamp, WhoWrote: write.WhoWrote})
				commitedKeys = append(commitedKeys, write.Key)
				s.readyTable[write.Key] = true
			}

			s.responseChan <- &msg.Response{Type: msg.Success, CommitedKeys: commitedKeys, ManagerNumber: s.managerNumber}

		case msg.Fail:
			// record this incident
			if s.isUp {
				s.failureRecords[command.Timestamp] = 0
			}
			// just turn off isUp
			s.isUp = false
			// isn't it weird to reply "success" for failing haha
			s.responseChan <- &msg.Response{Type: msg.Success}

		case msg.Recover:
			// record this incident
			if !s.isUp {
				key, found := getMaxKey(s.failureRecords)
				if found {
					s.failureRecords[key] = command.Timestamp
				} else {
					log.Fatal("ERROR: recover a site that didn't fail\n")
				}
			}
			// turn on isUp and set the replicated data to be not ready
			s.isUp = true
			for key := 2; key <= 20; key += 2 {
				s.readyTable[key] = false
			}

			// look through the waiting reads list
			waitingReads := make([]msg.Pair, 0)
			for _, pair := range *command.WaitingReads {
				versions := s.table[pair.Key]
				for _, version := range versions {
					if version.Timestamp == pair.Timestamp {
						waitingReads = append(waitingReads, version)
					}
				}
			}
			s.responseChan <- &msg.Response{Type: msg.Success, WaitingReads: &waitingReads}

		case msg.DumpRead:
			dumped := make([]msg.Pair, 0)
			for _, versions := range s.table {
				if versions != nil {
					dumped = append(dumped, versions[len(versions)-1])
				} else {
					dumped = append(dumped, msg.Pair{Key: -1, Value: -1})
				}
			}
			s.responseChan <- &msg.Response{Type: msg.DumpRead, PairsDumped: &dumped}
		default:
			// fmt.Println("ERROR: DM received an unknown command!")
			s.responseChan <- &msg.Response{Type: msg.Error}
		}
	}
}

// getMaxKey returns the largest key in the map.
// If the map is empty, it returns 0 and a boolean false.
func getMaxKey(m map[uint64]uint64) (uint64, bool) {
	var maxKey uint64
	var found bool

	for key := range m {
		if !found || key > maxKey {
			maxKey = key
			found = true
		}
	}

	return maxKey, found
}

func isOverlapping(failureRecords map[uint64]uint64, dmAccessTime, txnCommitTime uint64) bool {
	for failureStart, failureEnd := range failureRecords {
		if (failureStart <= txnCommitTime && failureStart >= dmAccessTime) ||
			(failureEnd >= dmAccessTime && failureEnd <= txnCommitTime) ||
			(failureStart <= dmAccessTime && failureEnd >= txnCommitTime) {
			return true
		}
	}
	return false
}
