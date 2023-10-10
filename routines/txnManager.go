package routines

import (
	"bufio"
	"fmt"
	"log"
	"minidb/msg"
	"os"
	"strconv"
	"strings"
)

const (
	NUMBER_OF_DMS = 10
)

type ongoingTxn struct {
	timeStart  uint64
	writes     map[int]msg.Pair
	dmAccessed [NUMBER_OF_DMS + 1]bool
}

type TxnManagerState struct {
	timeNow         uint64
	commitHistories [21][]uint64 // for enforcing "first commiter wins"
	ongoingTxns     map[int]ongoingTxn
	dmUpTable       [11]bool
	dmCommandChans  []chan *msg.Command
	dmResponseChans []chan *msg.Response
}

func NewTxnManagerAndDataManagers() TxnManagerState {
	// create & init new TM
	newTM := TxnManagerState{timeNow: 0, ongoingTxns: make(map[int]ongoingTxn, 0),
		dmCommandChans: make([]chan *msg.Command, 11), dmResponseChans: make([]chan *msg.Response, 11)}
	for i := 1; i <= NUMBER_OF_DMS; i++ {
		newTM.dmUpTable[i] = true
		newTM.dmCommandChans[i] = make(chan *msg.Command)
		newTM.dmResponseChans[i] = make(chan *msg.Response)
	}
	for i := 1; i <= 20; i++ {
		newTM.commitHistories[i] = []uint64{newTM.timeNow}
	}

	// create new DMs
	for i := 1; i <= NUMBER_OF_DMS; i++ {
		newDM := NewDataManager(i, newTM.dmCommandChans[i], newTM.dmResponseChans[i], newTM.timeNow)
		go newDM.DataManagerRoutine()
	}

	// make sure the first txn start at 1
	newTM.timeNow = 1

	return newTM
}

func (s *TxnManagerState) TxnManagerRoutine() {
	reader := bufio.NewReader(os.Stdin)

	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal("Error reading input:", err)
		}

		// for every command read, increment time!!
		s.timeNow++

		// remove any trailing whitespace
		command := strings.TrimSpace(input)
		// parse the command type and content
		cmdType := ""
		cmdContent := ""

		if openIndex := strings.Index(command, "("); openIndex != -1 && strings.HasSuffix(command, ")") {
			cmdType = command[:openIndex]
			cmdContent = command[openIndex+1 : len(command)-1]
		} else {
			log.Fatal("Unknown Command!!!")
		}

		switch cmdType {
		case "begin":
			// string processing
			txnNumberString := cmdContent[1:] // remove the "T"
			txnNumber, err := strconv.Atoi(txnNumberString)
			if err != nil {
				log.Fatal("Illegal Command!!!")
			}
			// check if txn already registered
			_, ok := s.ongoingTxns[txnNumber]
			if ok {
				log.Fatal("Txn Already Began!!!")
			}
			// register new txn
			s.ongoingTxns[txnNumber] = ongoingTxn{timeStart: s.timeNow, writes: make(map[int]msg.Pair, 0)}

		case "R":
			// string processing
			parts := strings.Split(cmdContent, ",")
			txnNumber, err1 := strconv.Atoi(parts[0][1:])
			key, err2 := strconv.Atoi(parts[1][1:])
			if err1 != nil || err2 != nil {
				log.Fatal("Illegal Command!!!")
			}

			// check if the txn exists and then perform the read
			txn, ok := s.ongoingTxns[txnNumber]
			if !ok {
				log.Fatal("Txn doesn't exist!!!")
			}
			// first enforce "read your own writes"
			// then try to read from DMs, try from site 1 to 10
			success := false
			var res *msg.Response
			if writtenPair, writtenExists := txn.writes[key]; writtenExists {
				// read success, print and then end this command
				fmt.Println("x", key, ": ", writtenPair.Value)
				continue
			} else {
				for i := 1; i <= NUMBER_OF_DMS; i++ {
					// NOTE: we can do load balancing with mod function here
					// but we don't have concurrency, so, forget about it
					s.dmCommandChans[i] <- &msg.Command{Type: msg.Read, Pair: msg.Pair{Key: key}, Timestamp: txn.timeStart}
					res = <-s.dmResponseChans[i]
					if res.Type == msg.Read {
						success = true
						break
					}
				}
			}
			// if no DM can perform the read right now, abort
			if !success {
				s.abort(txnNumber)
				continue
			}
			// read success, print and add to accessedDM
			fmt.Println("x", key, ": ", res.Pair.Value)
			txn.dmAccessed[res.ManagerNumber] = true
			s.ongoingTxns[txnNumber] = txn

		case "W":
			// string processing
			parts := strings.Split(cmdContent, ",")
			txnNumber, _ := strconv.Atoi(parts[0][1:])
			key, _ := strconv.Atoi(parts[1][1:])
			value, _ := strconv.Atoi(parts[2])
			// double check the transaction exists
			txn, ok := s.ongoingTxns[txnNumber]
			if !ok {
				log.Fatal("Txn doesn't exist!!!")
			}

			// NOTE: here we are supposed to write to all DMs that is up
			// or only one DM if the pair is not being replicated
			// but the writes should not be visible to other transactions
			// until this transaction commits, so we don't really write to
			// the data managers, we defer the actual writes to the commit

			// check if the target pair is replicated or not
			if isReplicated(key) {
				// add all nodes that are currently up into the dmAccessed
				for i := 1; i <= NUMBER_OF_DMS; i++ {
					txn.dmAccessed[i] = s.dmUpTable[i]
				}
			} else {
				// if the DM is up, record accessed, otherwise abort
				managerNumber := locateUnreplicated(key)
				if s.dmUpTable[managerNumber] {
					txn.dmAccessed[managerNumber] = true
				} else {
					s.abort(txnNumber)
					continue
				}
			}

			// record the writes, if there were earlier value written, simply overwrite
			txn.writes[key] = msg.Pair{Key: key, Value: value}

		case "dump":
			// just do dumpRead on every DM
			for i := 1; i < NUMBER_OF_DMS; i++ {
				s.dmCommandChans[i] <- &msg.Command{Type: msg.DumpRead}
				res := <-s.dmResponseChans[i]
				dumped := *res.PairsDumped
				dumpedLen := len(dumped)
				// print the results
				fmt.Print("site ", i, " - ")
				for j := 0; j < dumpedLen-1; j++ {
					fmt.Print("x", dumped[j].Key, ": ", dumped[j].Value, ", ")
				}
				// print the last one without ", " but "\n"
				fmt.Print("x", dumped[dumpedLen-1].Key, ": ", dumped[dumpedLen-1].Value, "\n")
			}

		case "end":
			// string processing
			txnNumberString := cmdContent[1:] // remove the "T"
			txnNumber, _ := strconv.Atoi(txnNumberString)
			// double check the transaction exists
			txn, ok := s.ongoingTxns[txnNumber]
			if !ok {
				log.Fatal("Txn doesn't exist!!!")
			}

			// enforcing the "first commiter wins" rule
			// for every written key, check if there are new commit history after this txn starts
			for _, write := range txn.writes {
				if s.commitHistories[write.Key][len(s.commitHistories[write.Key])-1] >= txn.timeStart {
					// someone else commited the value we wrote, so abort
					s.abort(txnNumber)
				}
			}

			// this txn is cleared, send Commit commands to DMs
			writesToCommit := make([]msg.Pair, 0)
			for _, write := range txn.writes {
				// update the timestamp!!
				write.Timestamp = s.timeNow
				writesToCommit = append(writesToCommit, write)
			}
			for i := 1; i <= NUMBER_OF_DMS; i++ {
				if s.dmUpTable[i] {
					s.dmCommandChans[i] <- &msg.Command{Type: msg.Commit, WritesToCommit: &writesToCommit, Timestamp: s.timeNow}
				}
			}
			for i := 1; i <= NUMBER_OF_DMS; i++ {
				if s.dmUpTable[i] {
					<-s.dmResponseChans[i]
				}
			}

			// record this commit to the commit histories
			for _, write := range txn.writes {
				history := s.commitHistories[write.Key]
				history = append(history, write.Timestamp)
				s.commitHistories[write.Key] = history
			}

		case "fail":
			// string processing
			managerNumber, _ := strconv.Atoi(cmdContent)
			s.dmCommandChans[managerNumber] <- &msg.Command{Type: msg.Fail}
			<-s.dmResponseChans[managerNumber]

		case "recover":
			// string processing
			managerNumber, _ := strconv.Atoi(cmdContent)
			s.dmCommandChans[managerNumber] <- &msg.Command{Type: msg.Recover}
			<-s.dmResponseChans[managerNumber]

		default:
			log.Fatal("Unknown Command!!!")
		}
	}
}

func (s *TxnManagerState) abort(txnNumber int) {
	_, ok := s.ongoingTxns[txnNumber]
	if !ok {
		log.Fatal("Txn doesn't exist!!!")
	}
	delete(s.ongoingTxns, txnNumber)
}

func isReplicated(key int) bool {
	return key%2 == 0
}

func locateUnreplicated(key int) int {
	if isReplicated(key) {
		log.Fatal("You should sleep more!!!")
	}
	return 1 + key%10
}
