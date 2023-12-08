// data_structures.go
package msg

type Pair struct {
	Key       int
	Value     int
	Timestamp uint64
	WhoWrote  int
}

type Type int

const (
	// general message types
	Begin Type = iota
	Read
	Write
	Dump
	End
	Fail
	Recover

	// message bewteen DMs and TM
	Commit
	DumpRead
	Success
	Error
)

type Command struct {
	Type           Type
	Pair           Pair
	TxnNumber      int
	SiteNumber     int
	Timestamp      uint64
	ExtraTimestamp uint64
	VersionNeeded  uint64
	WritesToCommit *[]Pair
	WaitingReads   *[]Pair
}

type Response struct {
	Type          Type
	Pair          Pair
	TxnNumber     int
	ManagerNumber int
	PairsDumped   *[]Pair
	CommitedKeys  []int
	WaitingReads  *[]Pair
}

type TxnManagerState struct {
}
