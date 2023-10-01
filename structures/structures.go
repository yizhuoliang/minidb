// data_structures.go
package structures

type Pair struct {
	Key       int
	Value     int
	Timestamp uint64
}

type Type int

const (
	Begin Type = iota
	Read
	Write
	Dump
	End
	Fail
	Recover
)

type Command struct {
	Type       Type
	Pair       Pair
	TxnNumber  int
	SiteNumber int
}

type Response struct {
	Type       Type
	Pair       Pair
	TxnNumber  int
	SiteNumber int
	Pairs      []Pair
}

type DataManagerState struct {
	Number        int
	IsUp          bool
	Table         [21]Pair
	ReadyTable    [21]bool
	CommandChan   chan Command
	ResponseChan  chan Response
	TimestampChan chan uint64
}

type TxnManagerState struct {
}
