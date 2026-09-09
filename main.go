package main

import "fmt"

// conn comes up on the loop, shows its cover, and calls hello. Everything
// it was is in the history, and comes back piece by piece, in the form it
// is wanted in.
func main() {
	p := plain
	if stdoutIsTerminal() {
		p = colored
	}
	fmt.Print(banner(p))
}

const greeting = "hello from the conn"
