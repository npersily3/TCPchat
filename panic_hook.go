package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
)

// Shadows the built-in panic. Prints the value and full stack trace,
// fires INT3 so an attached debugger breaks at this point, then exits.
// Only intercepts panics originating in this package — runtime/library
// panics (e.g. nil deref, out of bounds) go through the normal runtime path.
func panic(v any) {
	fmt.Fprintf(os.Stderr, "\npanic: %v\n\n", v)
	debug.PrintStack()
	runtime.Breakpoint() // INT3 — debugger breaks here, check the stack above for origin
	os.Exit(2)
}
