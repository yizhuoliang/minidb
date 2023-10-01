package routines

import (
	"fmt"
	"minidb/structures"
)

func DataManager(state structures.DataManagerState) {
	for {
		command, ok := <-state.CommandChan
		if !ok {
			fmt.Println("ERROR: CommandChan closed!")
			break
		}

		switch command.Type {
		case structures.Begin:
		case structures.Read:
		case structures.Write:
		case structures.Dump:
		case structures.End:
		case structures.Fail:
		case structures.Recover:
		default:
			fmt.Println("ERROR: DM received an unknown command!")
		}
	}
}
