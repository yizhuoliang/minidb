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
	state        state
	txnNumber    int
	timeStart    uint64
	writes       map[int]msg.Pair // map from key to pair
	reads        map[int]msg.Pair // map from key to pair
	readFroms    map[int]bool     // NOTE: map from txnNumber to bool
	dmAccessed   [NUMBER_OF_DMS + 1]bool
	reason       string
	dmFailed     [NUMBER_OF_DMS + 1]bool // DMs that failed during txn execution
	isWaiting    bool
	waitingSites []int
	waitingKey   int
	waitingTime  uint64
}

type commitHistory struct {
	timestamp    uint64
	txnNumber    int
	siteCommited []int
}

type TxnManagerState struct {
	timeNow         uint64
	commitHistories [21][]commitHistory // for enforcing "first commiter wins"
	txnRecords      map[int]txnRecord
	dmUpTable       [11]bool
	dmCommandChans  []chan *msg.Command
	dmResponseChans []chan *msg.Response
	serialGraph     *graph.Graph
	waitingCommands []string
	waitingTxns     map[int]bool
}

func NewTxnManagerAndDataManagers() TxnManagerState {
	// create & init new TM
	newTM := TxnManagerState{timeNow: 0, txnRecords: make(map[int]txnRecord, 0), waitingCommands: make([]string, 0),
		dmCommandChans: make([]chan *msg.Command, 11), dmResponseChans: make([]chan *msg.Response, 11)}
	for i := 1; i <= NUMBER_OF_DMS; i++ {
		newTM.dmUpTable[i] = true
		newTM.dmCommandChans[i] = make(chan *msg.Command)
		newTM.dmResponseChans[i] = make(chan *msg.Response)
	}
	for i := 1; i <= 20; i++ {
		historyEntry := commitHistory{timestamp: 0, siteCommited: make([]int, 0)}
		if isReplicated(i) {
			for j := 1; j <= NUMBER_OF_DMS; j++ {
				historyEntry.siteCommited = append(historyEntry.siteCommited, j)
			}
		} else {
			historyEntry.siteCommited = append(historyEntry.siteCommited, locateUnreplicated(i))
		}
		newTM.commitHistories[i] = []commitHistory{historyEntry}
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

func (s *TxnManagerState) TxnManagerRoutine(inputFilePath string) {
	// open the file to read from
	inFile, inErr := os.Open(inputFilePath)
	if inErr != nil {
		log.Fatal(inErr)
	}
	defer inFile.Close()

	// Open the file for writing, appending, and create it if it doesn't exist
	file, err := os.OpenFile("output.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("failed opening file: %s", err)
	}
	defer file.Close()

	// Redirect the output of fmt.Printf to the file
	oldStdout := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = oldStdout }()

	// print a divider to output file
	fmt.Printf("\n----------------------------\n")

	// Create a Scanner to read the file
	scanner := bufio.NewScanner(inFile)

	for {
		if !scanner.Scan() {
			break // No more lines, terminates
		}

		input := scanner.Text()
		// remove comments if there is any
		index := strings.Index(input, "//")
		if index != -1 {
			input = input[:index]
		}
		// Check for blank line
		if strings.TrimSpace(input) == "" {
			continue // Skip blank or comment lines
		}

		// for every command, increment time!!
		s.timeNow++

		// remove any trailing whitespace
		command := strings.TrimSpace(input)
		command = strings.ReplaceAll(command, " ", "") // remove any empty strings
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
			// we looup what is the last commited version of this key before this txn starts
			versionNeeded := uint64(0)
			var dmNeeded []int = make([]int, 0)
			for _, history := range s.commitHistories[key] {
				if history.timestamp >= txn.timeStart {
					break
				}
				versionNeeded = history.timestamp
				dmNeeded = history.siteCommited
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
					s.dmCommandChans[i] <- &msg.Command{Type: msg.Read, Pair: msg.Pair{Key: key}, ExtraTimestamp: s.timeNow, VersionNeeded: versionNeeded}
					res = <-s.dmResponseChans[i]
					if res.Type == msg.Read {
						success = true
						break
					}
				}
			}
			// if no DM can perform the read right now, decide on wait or abort
			if !success {
				// look at the version needed's commit history
				// see if one of the DM failed finished during this txn's execution
				waiting := false
				if dmNeeded == nil {
					s.abort(txnNumber, true, "no available DM can read from.")
					continue
				}
				for _, dmn := range dmNeeded {
					if txn.dmFailed[dmn] && s.dmUpTable[dmn] == false {
						// now we can wait!
						if txn.isWaiting == false {
							fmt.Printf("T%d is Waiting\n", txnNumber)
							txn.isWaiting = true
							txn.waitingSites = make([]int, 0)
							txn.waitingKey = key
							txn.waitingTime = versionNeeded
						}
						waiting = true
						txn.waitingSites = append(txn.waitingSites, dmn)
					}
				}
				if waiting {
					s.txnRecords[txnNumber] = txn
					continue
				}
				// if can't wait, abort
				//fmt.Print("Place 2\n")
				s.abort(txnNumber, true, "no available DM can read from.")
				continue
			}
			// read success, print and add to accessedDM
			fmt.Printf("T%d Reads (x%d = %d)\n", txnNumber, key, res.Pair.Value)
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
			// the data managers, we defer the actual writes to the commit,
			// but the DMs still record the access history

			// check if the target pair is replicated or not
			if isReplicated(key) {
				fmt.Printf("T%d Writes uncommited value (x%d = %d) to sites: ", txnNumber, key, value)
				// add all nodes that are currently up into the dmAccessed
				for i := 1; i <= NUMBER_OF_DMS; i++ {
					txn.dmAccessed[i] = s.dmUpTable[i]
					s.dmCommandChans[i] <- &msg.Command{Type: msg.Write, TxnNumber: txnNumber, Timestamp: s.timeNow, Pair: msg.Pair{Key: key, Value: value}}
					res := <-s.dmResponseChans[i]
					if res.Type == msg.Success {
						fmt.Printf("%d ", i)
					}
				}
				fmt.Println()
			} else {
				// if the DM is up, record accessed, otherwise abort
				managerNumber := locateUnreplicated(key)
				if s.dmUpTable[managerNumber] {
					txn.dmAccessed[managerNumber] = true
					s.dmCommandChans[managerNumber] <- &msg.Command{Type: msg.Write, TxnNumber: txnNumber, Timestamp: s.timeNow, Pair: msg.Pair{Key: key, Value: value}}
					<-s.dmResponseChans[managerNumber]
				} else {
					//fmt.Print("Place 3\n")
					s.abort(txnNumber, true, "no available DM can write to.")
					continue
				}
				fmt.Printf("T%d Writes (x%d = %d) to site %d\n", txnNumber, key, value, managerNumber)
			}

			// record the writes, if there were earlier value written, simply overwrite
			txn.writes[key] = msg.Pair{Key: key, Value: value}
			s.txnRecords[txnNumber] = txn

		case "dump":
			// just do dumpRead on every DM
			fmt.Print("keys   - ")
			for i := 1; i <= 20; i++ {
				fmt.Printf("x%-5d", i)
			}
			fmt.Println()
			for i := 1; i <= NUMBER_OF_DMS; i++ {
				s.dmCommandChans[i] <- &msg.Command{Type: msg.DumpRead}
				res := <-s.dmResponseChans[i]
				dumped := *res.PairsDumped
				dumpedLen := len(dumped)
				// print the results
				fmt.Printf("site%2d - ", i)
				for j := 1; j < dumpedLen; j++ {
					if dumped[j].Key != -1 {
						fmt.Printf("%-5s ", fmt.Sprint(dumped[j].Value))
					} else {
						fmt.Printf("NA    ")
					}
				}
				fmt.Println()
				// print the last one without ", " but "\n"
				// if dumped[dumpedLen-1].Key != -1 {
				// 	fmt.Printf("x%d: %4s\n", dumped[dumpedLen-1].Key, fmt.Sprint(dumped[dumpedLen-1].Value))
				// } else {
				// 	fmt.Printf("x%d: %4s\n", dumpedLen, "NA")
				// }
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
				// if the txn is already aborted, just report aborted again
				//fmt.Println("Place 7")
				s.abort(txnNumber, false, "")
				continue
			} else if txn.isWaiting == true {
				fmt.Printf("ERROR! T%d is still waiting, cant be commited!\n", txnNumber)
				continue
			}

			// enforcing the "first commiter wins" rule
			// for every written key, check if there are new commit history after this txn starts
			needAbort := false
			for _, write := range txn.writes {
				if s.commitHistories[write.Key][len(s.commitHistories[write.Key])-1].timestamp >= txn.timeStart {
					// someone else commited the value we wrote, so abort
					//fmt.Print("Place 4\n")
					s.abort(txnNumber, false, "first commiter wins.")
					needAbort = true
					break
				}
			}
			if needAbort {
				continue
			}

			// enforcing SSI invariant by the graph
			if !s.checkAndUpdateSerializationGraph(txnNumber) {
				//fmt.Print("Place 5\n")
				s.abort(txnNumber, false, "consecutive rw edges found.")
				continue
			}

			// this txn is cleared, send Commit commands to DMs
			writesToCommit := make([]msg.Pair, 0)
			keyToCommitedSites := make(map[int][]int, 0)
			for _, write := range txn.writes {
				// update the timestamp!!
				write.Timestamp = s.timeNow
				write.WhoWrote = txnNumber
				txn.writes[write.Key] = write
				writesToCommit = append(writesToCommit, write)
				keyToCommitedSites[write.Key] = make([]int, 0)
			}
			for i := 1; i <= NUMBER_OF_DMS; i++ {
				if s.dmUpTable[i] {
					s.dmCommandChans[i] <- &msg.Command{Type: msg.Commit, WritesToCommit: &writesToCommit, Timestamp: s.timeNow, TxnNumber: txnNumber}
				}
			}
			for i := 1; i <= NUMBER_OF_DMS; i++ {
				if s.dmUpTable[i] {
					res := <-s.dmResponseChans[i]
					if res.Type == msg.Success {
						for _, commitedKey := range res.CommitedKeys {
							sites, ok := keyToCommitedSites[commitedKey]
							if ok {
								sites = append(sites, res.ManagerNumber)
							}
							keyToCommitedSites[commitedKey] = sites
						}
					}
				}
			}

			// record this commit to the commit histories
			for _, write := range txn.writes {
				history := s.commitHistories[write.Key]
				historyEntry := commitHistory{timestamp: write.Timestamp, txnNumber: txnNumber, siteCommited: make([]int, 0)}
				for _, site := range keyToCommitedSites[write.Key] {
					historyEntry.siteCommited = append(historyEntry.siteCommited, site)
				}
				history = append(history, historyEntry)
				s.commitHistories[write.Key] = history
			}
			txn.state = commited
			s.txnRecords[txnNumber] = txn
			fmt.Printf("T%d Commited\n", txnNumber)

		case "fail":
			// string processing
			managerNumber, _ := strconv.Atoi(cmdContent)
			s.dmCommandChans[managerNumber] <- &msg.Command{Type: msg.Fail, Timestamp: s.timeNow}
			<-s.dmResponseChans[managerNumber]

			// mark this DM as down
			s.dmUpTable[managerNumber] = false

			// abort all txns that accessed this DM
			for key, txn := range s.txnRecords {
				if txn.state == ongoing && txn.dmAccessed[managerNumber] {
					// fmt.Print("Place 1\n")
					s.abort(txn.txnNumber, true, "accessed site crashed.")
				} else if txn.state == ongoing {
					txn.dmFailed[managerNumber] = true
					s.txnRecords[key] = txn
				}
			}

		case "recover":
			// string processing
			managerNumber, _ := strconv.Atoi(cmdContent)
			waitingReads := make([]msg.Pair, 0)
			for _, txn := range s.txnRecords {
				if txn.state == ongoing && txn.isWaiting {
					for _, dmn := range txn.waitingSites {
						if dmn == managerNumber {
							waitingReads = append(waitingReads, msg.Pair{Key: txn.waitingKey, Timestamp: txn.waitingTime})
							break
						}
					}
				}
			}
			s.dmCommandChans[managerNumber] <- &msg.Command{Type: msg.Recover, Timestamp: s.timeNow, WaitingReads: &waitingReads}
			res := <-s.dmResponseChans[managerNumber]
			for _, txn := range s.txnRecords {
				if txn.state == ongoing && txn.isWaiting {
					for _, pair := range *res.WaitingReads {
						if pair.Key == txn.waitingKey && pair.Timestamp == txn.waitingTime {
							// this txn can continue!
							fmt.Printf("T%d Reads (x%d = %d)\n", txn.txnNumber, pair.Key, pair.Value)
							txn.dmAccessed[managerNumber] = true
							txn.readFroms[pair.WhoWrote] = true
							txn.isWaiting = false
							s.txnRecords[txn.txnNumber] = txn
						}
					}
				}
			}
			// mark this DM as up
			s.dmUpTable[managerNumber] = true

		case "flush":
			// return to main so that the output file is closed
			// and the database will be re-initialized
			// print a divider to output file
			fmt.Printf("\n----------------------------\n")
			newState := NewTxnManagerAndDataManagers()
			s = &newState

		default:
			log.Fatal("Unknown Command!!!")
		}
		// fmt.Println(s.txnRecords)
	}
}

func (s *TxnManagerState) abort(txnNumber int, silent bool, reason string) {
	_, ok := s.txnRecords[txnNumber]
	if !ok {
		log.Fatal("Txn doesn't exist!!!")
	}

	record := s.txnRecords[txnNumber]
	record.reason += reason
	record.state = aborted
	s.txnRecords[txnNumber] = record
	if !silent {
		fmt.Printf("T%d Aborted: %s\n", txnNumber, record.reason)
	}
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
		// fmt.Printf("Aborting T%d because cycle with rw - rw\n", txnNumber)
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
