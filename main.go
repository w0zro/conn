package main

import (
	"fmt"
	"time"
)

// conn comes up: the start-up screen is painted a row at a time, the
// station reports itself, and conn calls hello from the loop. Everything
// it was is in the history, and comes back piece by piece, in the form it
// is wanted in.
func main() {
	p, width, pace := plain, 80, time.Duration(0)
	if stdoutIsTerminal() {
		p, width, pace = colored, terminalWidth(), 28*time.Millisecond
	}
	for _, row := range screen(p, stationReport(), width) {
		fmt.Println(row)
		time.Sleep(pace)
	}
}

const greeting = "hello from the conn"
