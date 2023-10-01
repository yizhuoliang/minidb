// data_structures.go
package msg

type Pair struct {
	Key        int
	Value      int
	Timestamp  uint64
	Replicated bool
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
	Type            Type
	Pair            Pair
	TxnNumber       int
	SiteNumber      int
	CommitTimestamp uint64
	WritesToCommit  *[]Pair
}

type Response struct {
	Type        Type
	Pair        Pair
	TxnNumber   int
	SiteNumber  int
	PairsDumped *[]Pair
}

type TxnManagerState struct {
}
