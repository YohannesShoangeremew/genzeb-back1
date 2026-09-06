package utils

import (
	"log"
	"runtime/debug"
)

// RecoverPanic recovers a panicking goroutine and logs it with a stack trace,
// so one failed operation can never crash the whole process. Use as the FIRST
// deferred call in any background goroutine:
//
//	go func() {
//	    defer utils.RecoverPanic("draw-numbers")
//	    ...
//	}()
//
// Go's default is to terminate the entire program on an unrecovered panic in
// ANY goroutine — gin.Recovery only protects HTTP request goroutines, not the
// game loop, tickers, bot filler, or delivery goroutines. This is the guard
// that keeps a single bad game from taking every player offline.
func RecoverPanic(label string) {
	if r := recover(); r != nil {
		log.Printf("[PANIC RECOVERED] %s: %v\n%s", label, r, debug.Stack())
	}
}

// GoSafe launches fn in a goroutine that recovers from panics.
func GoSafe(label string, fn func()) {
	go func() {
		defer RecoverPanic(label)
		fn()
	}()
}
