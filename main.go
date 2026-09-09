package main

import (
	"fmt"
	"time"
)

// conn comes up: the start-up screen is written a row at a time, the way
// a screen was painted down a serial line, the station reports itself,
// and conn calls hello from the loop. Everything it was is in the
// history, and comes back piece by piece, in the form it is wanted in.
func main() {
	width, pace := screenCols, time.Duration(0)
	if stdoutIsTerminal() {
		width, pace = terminalWidth(), 30*time.Millisecond
		bright, normal = "\x1b[1m", "\x1b[0m"
	}
	for _, row := range screen(stationReport(), width) {
		fmt.Println(row)
		time.Sleep(pace)
	}
}

const greeting = "hello from the conn"
