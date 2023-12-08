package main

import (
	"fmt"
	"minidb/routines"
	"os"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "" {
		fmt.Println("ERROR! An argument indicating the input file path is required!!")
		os.Exit(1)
	}

	inputFilePath := os.Args[1]

	// simply launch TM and DMs
	txnManager := routines.NewTxnManagerAndDataManagers()

	// then start the TM
	txnManager.TxnManagerRoutine(inputFilePath)
}
