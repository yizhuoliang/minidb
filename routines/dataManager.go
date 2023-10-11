package routines

import (
	"log"
	"minidb/msg"
)

type DataManagerState struct {
	managerNumber int
	isUp          bool
	table         [21][]msg.Pair
	readyTable    [21]bool
	commandChan   chan *msg.Command
	responseChan  chan *msg.Response
}

func NewDataManager(managerNumber int, commandChan chan *msg.Command, responseChan chan *msg.Response, initTime uint64) DataManagerState {
	// build a new DM state
	newDM := DataManagerState{
		managerNumber: managerNumber,
		isUp:          true,
		commandChan:   commandChan,
		responseChan:  responseChan,
		// UncommittedTxnWrites: make(map[int][]msg.Pair),
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
					if versions[i].Timestamp < command.Timestamp {
						s.responseChan <- &msg.Response{Type: msg.Read, ManagerNumber: s.managerNumber, Pair: versions[i]}
						served = true
						break
					}
				}
			}
			if !served {
				s.responseChan <- &msg.Response{Type: msg.Error}
			}

		// case msg.Write:
		// 	if !s.isUp {
		// 		s.responseChan <- &msg.Response{Type: msg.Error}
		// 	}

		// 	// append the write operation to the corresponding slice
		// 	if writes, ok := s.UncommittedTxnWrites[command.TxnNumber]; ok {
		// 		// If exists, append newPair to the existing slice
		// 		writes = append(writes, command.Pair)
		// 	} else {
		// 		s.UncommittedTxnWrites[command.TxnNumber] = []msg.Pair{command.Pair}
		// 	}
		// 	s.responseChan <- &msg.Response{Type: msg.Success}

		case msg.Commit:
			if !s.isUp {
				s.responseChan <- &msg.Response{Type: msg.Error}
				continue
			}

			for _, write := range *command.WritesToCommit {
				s.table[write.Key] = append(s.table[write.Key], msg.Pair{Key: write.Key, Value: write.Value, Timestamp: command.Timestamp, WhoWrote: write.WhoWrote})
				s.readyTable[write.Key] = true
			}

			s.responseChan <- &msg.Response{Type: msg.Success}

		case msg.Fail:
			// just turn off isUp
			s.isUp = false
			// isn't it weird to reply "success" for failing haha
			s.responseChan <- &msg.Response{Type: msg.Success}

		case msg.Recover:
			// turn on isUp and set the replicated data to be not ready
			s.isUp = true
			for key := 2; key <= 20; key++ {
				s.readyTable[key] = false
			}
			s.responseChan <- &msg.Response{Type: msg.Success}

		case msg.DumpRead:
			dumped := make([]msg.Pair, 0)
			for _, versions := range s.table {
				if versions != nil {
					dumped = append(dumped, versions[len(versions)-1])
				}
			}
			s.responseChan <- &msg.Response{Type: msg.DumpRead, PairsDumped: &dumped}
		default:
			// fmt.Println("ERROR: DM received an unknown command!")
			s.responseChan <- &msg.Response{Type: msg.Error}
		}
	}
}
