package main

import "fmt"

// conn says hello. Everything it was is in the history, and comes back
// piece by piece, in the form it is wanted in.
func main() {
	fmt.Println(greeting)
}

const greeting = "hello from the conn"
