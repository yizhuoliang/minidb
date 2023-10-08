package main

import "minidb/routines"

func main() {
	// simply launch TM and DMs
	txnManager := routines.NewTxnManagerAndDataManagers()

	// then start the TM
	txnManager.TxnManagerRoutine()
}
