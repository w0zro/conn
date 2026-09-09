package main

import "testing"

func TestConnSaysHello(t *testing.T) {
	if greeting != "hello from the conn" {
		t.Errorf("greeting = %q", greeting)
	}
}
