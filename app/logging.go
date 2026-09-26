package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var logFile *os.File

// earlyLog holds lines written before shell.log is opened (update recovery,
// settings, the restored-payload decision) so they land at the top of the
// session's log instead of vanishing.
var earlyLog []string

// logMu serializes logf with opening and closing the log: bridges and the
// agent log from their own goroutines, and earlyLog is a plain slice.
var logMu sync.Mutex

func logf(format string, a ...any) {
	line := fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		fmt.Fprintln(logFile, line)
	} else if len(earlyLog) < 200 {
		earlyLog = append(earlyLog, line)
	}
}

// openLog starts writing the log to f, first flushing the lines logged before
// it was open.
func openLog(f *os.File) {
	logMu.Lock()
	defer logMu.Unlock()
	logFile = f
	for _, line := range earlyLog {
		fmt.Fprintln(f, line)
	}
	earlyLog = nil
}

// closeLog closes the log file; later lines collect in earlyLog again.
func closeLog() {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}
