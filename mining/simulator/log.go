package main

import (
	"fmt"
	"github.com/daglabs/btcd/logs"
	"github.com/daglabs/btcd/rpcclient"
	"github.com/daglabs/btcd/util/panics"
	"os"
)

var (
	backendLog = logs.NewBackend()
	log        = backendLog.Logger("MNSM")
	spawn      = panics.GoroutineWrapperFunc(log, backendLog)
)

func initLog(logFile, errLogFile string) {
	err := backendLog.AddLogFile(logFile, logs.LevelTrace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding log file %s as log rotator for level %s: %s", logFile, logs.LevelTrace, err)
		os.Exit(1)
	}
	err = backendLog.AddLogFile(errLogFile, logs.LevelWarn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding log file %s as log rotator for level %s: %s", errLogFile, logs.LevelWarn, err)
		os.Exit(1)
	}
}

func enableRPCLogging() {
	rpclog := backendLog.Logger("RPCC")
	rpclog.SetLevel(logs.LevelTrace)
	rpcclient.UseLogger(rpclog, backendLog)
}