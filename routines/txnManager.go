package routines

import (
	"bufio"
	"fmt"
	"log"
	"minidb/graph"
	"minidb/msg"
	"os"
	"strconv"
	"strings"
)

const (
	NUMBER_OF_DMS = 10
)

/*
	TODO: for every ongoing txn, add a record of reads
	for commit history, add who commited
	in graph, add a map from txn number to *node
	in the end, check the four types of edges
*/

type state int

const (
	// general message types
	ongoing state = iota
	aborted
	commited
)

type txnRecord struct {
	state      state
	txnNumber  int
	timeStart  uint64
	writes     map[int]msg.Pair // map from key to pair
	reads      map[int]msg.Pair // map from key to pair
	readFroms  map[int]bool     // NOTE: map from txnNumber to bool
	dmAccessed [NUMBER_OF_DMS + 1]bool
}

type commitHistory struct {
	timestamp uint64
	txnNumber int
}

type TxnManagerState struct {
	timeNow         uint64
	commitHistories [21][]commitHistory // for enforcing "first commiter wins"
	txnRecords      map[int]txnRecord
	dmUpTable       [11]bool
	dmCommandChans  []chan *msg.Command
	dmResponseChans []chan *msg.Response
	serialGraph     *graph.Graph
}

func NewTxnManagerAndDataManagers() TxnManagerState {
	// create & init new TM
	newTM := TxnManagerState{timeNow: 0, txnRecords: make(map[int]txnRecord, 0),
		dmCommandChans: make([]chan *msg.Command, 11), dmResponseChans: make([]chan *msg.Response, 11)}
	for i := 1; i <= NUMBER_OF_DMS; i++ {
		newTM.dmUpTable[i] = true
		newTM.dmCommandChans[i] = make(chan *msg.Command)
		newTM.dmResponseChans[i] = make(chan *msg.Response)
	}
	for i := 1; i <= 20; i++ {
		newTM.commitHistories[i] = []commitHistory{{timestamp: newTM.timeNow, txnNumber: 0}}
	}

	// create new DMs
	for i := 1; i <= NUMBER_OF_DMS; i++ {
		newDM := NewDataManager(i, newTM.dmCommandChans[i], newTM.dmResponseChans[i], newTM.timeNow)
		go newDM.DataManagerRoutine()
	}

	// init the graph
	newTM.serialGraph = graph.NewGraph()

	// make sure the first txn start at 1
	newTM.timeNow = 1

	return newTM
}

func (s *TxnManagerState) TxnManagerRoutine() {

	for {
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal("Error reading input:", err)
		}

		// for every command, increment time!!
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
			_, ok := s.txnRecords[txnNumber]
			if ok {
				log.Fatal("Txn Already Began!!!")
			}
			// register new txn
			s.txnRecords[txnNumber] = txnRecord{state: ongoing, txnNumber: txnNumber, timeStart: s.timeNow, writes: make(map[int]msg.Pair, 0), reads: make(map[int]msg.Pair, 0), readFroms: make(map[int]bool, 0)}
			// add node to the serialization graph
			s.serialGraph.AddNode(txnNumber)

		case "R":
			// string processing
			parts := strings.Split(cmdContent, ",")
			txnNumber, err1 := strconv.Atoi(parts[0][1:])
			key, err2 := strconv.Atoi(parts[1][1:])
			if err1 != nil || err2 != nil {
				log.Fatal("Illegal Command!!!")
			}

			// check if the txn exists and then perform the read
			txn, ok := s.txnRecords[txnNumber]
			if !ok {
				log.Fatal("Txn doesn't exist!!!")
			}
			// first enforce "read your own writes"
			// then try to read from DMs, try from site 1 to 10
			success := false
			var res *msg.Response
			if writtenPair, writtenExists := txn.writes[key]; writtenExists {
				// read success, print and then end this command
				fmt.Printf("x%d: %d\n", key, writtenPair.Value)
				// just continue, skip any book keeping updates
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
			// update the bookkeeping stuff
			txn.reads[res.Pair.Key] = res.Pair
			txn.readFroms[res.Pair.WhoWrote] = true
			txn.dmAccessed[res.ManagerNumber] = true
			s.txnRecords[txnNumber] = txn

		case "W":
			// string processing
			parts := strings.Split(cmdContent, ",")
			txnNumber, _ := strconv.Atoi(parts[0][1:])
			key, _ := strconv.Atoi(parts[1][1:])
			value, _ := strconv.Atoi(parts[2])
			// double check the transaction exists
			txn, ok := s.txnRecords[txnNumber]
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
			s.txnRecords[txnNumber] = txn

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
			txn, ok := s.txnRecords[txnNumber]
			if !ok {
				log.Fatal("Txn doesn't exist!!!")
			} else if txn.state == aborted {
				// if the txn is already aborted, just remove that
				continue
			}

			// enforcing the "first commiter wins" rule
			// for every written key, check if there are new commit history after this txn starts
			needAbort := false
			for _, write := range txn.writes {
				if s.commitHistories[write.Key][len(s.commitHistories[write.Key])-1].timestamp >= txn.timeStart {
					// someone else commited the value we wrote, so abort
					s.abort(txnNumber)
					needAbort = true
				}
			}
			if needAbort {
				continue
			}

			// enforcing SSI invariant by the graph
			if !s.checkAndUpdateSerializationGraph(txnNumber) {
				s.abort(txnNumber)
				continue
			}

			// this txn is cleared, send Commit commands to DMs
			writesToCommit := make([]msg.Pair, 0)
			for _, write := range txn.writes {
				// update the timestamp!!
				write.Timestamp = s.timeNow
				write.WhoWrote = txnNumber
				txn.writes[write.Key] = write
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
				history = append(history, commitHistory{timestamp: write.Timestamp, txnNumber: txnNumber})
				s.commitHistories[write.Key] = history
			}
			txn.state = commited
			s.txnRecords[txnNumber] = txn
			// fmt.Print(s.commitHistories)
			// fmt.Printf("COMMIT T%d", txnNumber)

		case "fail":
			// string processing
			managerNumber, _ := strconv.Atoi(cmdContent)
			s.dmCommandChans[managerNumber] <- &msg.Command{Type: msg.Fail}
			<-s.dmResponseChans[managerNumber]

			// mark this DM as down
			s.dmUpTable[managerNumber] = false

			// abort all txns that accessed this DM
			for _, txn := range s.txnRecords {
				if txn.state == ongoing && txn.dmAccessed[managerNumber] {
					s.abort(txn.txnNumber)
				}
			}

		case "recover":
			// string processing
			managerNumber, _ := strconv.Atoi(cmdContent)
			// fmt.Println(cmdContent)
			s.dmCommandChans[managerNumber] <- &msg.Command{Type: msg.Recover}
			<-s.dmResponseChans[managerNumber]
			// mark this DM as up
			s.dmUpTable[managerNumber] = true

		default:
			log.Fatal("Unknown Command!!!")
		}
		// fmt.Println(s.txnRecords)
	}
}

func (s *TxnManagerState) abort(txnNumber int) {
	_, ok := s.txnRecords[txnNumber]
	if !ok {
		log.Fatal("Txn doesn't exist!!!")
	}

	record := s.txnRecords[txnNumber]
	record.state = aborted
	s.txnRecords[txnNumber] = record
	fmt.Printf("Aborting T%d \n", txnNumber)
}

func (s *TxnManagerState) checkAndUpdateSerializationGraph(txnNumber int) bool {
	thisTxn := s.txnRecords[txnNumber]
	// first try to add the edges
	copySerialGraph := *s.serialGraph
	serialGraph := &copySerialGraph
	for _, otherTxn := range s.txnRecords {
		if otherTxn.state == aborted || otherTxn.txnNumber == txnNumber {
			continue
		} else if otherTxn.state == commited {
			// WW EDGES
			for _, thisWrite := range thisTxn.writes {
				if _, ok := otherTxn.writes[thisWrite.Key]; ok {
					serialGraph.AddEdge(otherTxn.txnNumber, txnNumber, "ww")
				}
			}

			// WR EDGES
			if thisTxn.readFroms[otherTxn.txnNumber] {
				serialGraph.AddEdge(otherTxn.txnNumber, txnNumber, "wr")
			}
		} else { // when otherTxn is ongoing
			// RW EDGES, thisTxn write
			for _, thisWrite := range thisTxn.writes {
				if _, ok := otherTxn.reads[thisWrite.Key]; ok {
					serialGraph.AddEdge(otherTxn.txnNumber, txnNumber, "rw")
				}
			}
			// RW EDGES,
			for _, thisRead := range thisTxn.reads {
				if _, ok := otherTxn.writes[thisRead.Key]; ok {
					serialGraph.AddEdge(txnNumber, otherTxn.txnNumber, "rw")
				}
			}
		}
	}
	// check & update
	if serialGraph.HasCycleWithConsecutiveRWEdges() {
		fmt.Printf("Aborting T%d because cycle with rw - rw\n", txnNumber)
		return false
	} else {
		s.serialGraph = serialGraph
		return true
	}
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
