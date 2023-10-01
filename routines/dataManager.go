package routines

import (
	"fmt"
	"minidb/msg"
)

type DataManagerState struct {
	managerNumber        int
	isUp                 bool
	table                [21]*msg.Pair
	readyTable           [21]bool
	commandChan          chan *msg.Command
	responseChan         chan *msg.Response
	UncommittedTxnWrites map[int][]msg.Pair
}

func NewDataManager(managerNumber int, isUp bool, commandChan chan *msg.Command, responseChan chan *msg.Response, initTime uint64) DataManagerState {
	// build a new DM state
	newDM := DataManagerState{
		managerNumber: managerNumber,
		isUp:          isUp,
		commandChan:   commandChan,
		responseChan:  responseChan,
		// UncommittedTxnWrites: make(map[int][]msg.Pair),
	}

	// initialize the data tables
	for key := 2; key <= 20; key++ {
		newDM.table[key] = &msg.Pair{Key: key, Value: key * 10, Timestamp: initTime, Replicated: true}
		newDM.readyTable[key] = true
	}
	// only even indexed sites have monopolized keys
	if managerNumber%2 == 0 {
		key1 := managerNumber - 1
		key2 := managerNumber + 9
		newDM.table[key1] = &msg.Pair{Key: key1, Value: key1 * 10, Timestamp: initTime, Replicated: false}
		newDM.table[key2] = &msg.Pair{Key: key2, Value: key2 * 10, Timestamp: initTime, Replicated: false}
		newDM.readyTable[key1] = true
		newDM.readyTable[key2] = true
	}

	return newDM
}

func (s *DataManagerState) DataManagerRoutine() {
	for {
		command, ok := <-s.commandChan
		if !ok {
			fmt.Println("ERROR: commandChan closed!")
			break
		}

		switch command.Type {
		case msg.Read:
			// if down, always respond error
			if !s.isUp {
				s.responseChan <- &msg.Response{Type: msg.Error}
			}

			key := command.Pair.Key
			if s.readyTable[key] && s.table[key] != nil {
				s.responseChan <- &msg.Response{Type: msg.Read, Pair: *s.table[key]}
			} else {
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
			}

			for _, write := range *command.WritesToCommit {
				s.table[write.Key] = &msg.Pair{Key: write.Key, Value: write.Value, Timestamp: command.CommitTimestamp, Replicated: write.Replicated}
				s.readyTable[write.Key] = true
			}

			s.responseChan <- &msg.Response{Type: msg.Success}

		case msg.Fail:
			// just turn off isUp
			s.isUp = false

		case msg.Recover:
			// turn on isUp and set the replicated data to be not ready
			s.isUp = true
			for key := 2; key <= 20; key++ {
				s.readyTable[key] = false
			}
			s.responseChan <- &msg.Response{Type: msg.Success}

		case msg.DumpRead:
			dumped := make([]msg.Pair, 0)
			for _, pair := range s.table {
				if pair != nil {
					dumped = append(dumped, *pair)
				}
			}
			s.responseChan <- &msg.Response{Type: msg.DumpRead, PairsDumped: &dumped}
		default:
			// fmt.Println("ERROR: DM received an unknown command!")
			s.responseChan <- &msg.Response{Type: msg.Error}
		}
	}
}
