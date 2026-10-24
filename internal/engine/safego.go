package engine

import "fmt"

// Go runs fn on a goroutine of its own and keeps a panic in it from ending the program: the panic is logged, named by
// what, and only that goroutine ends (the check loop already does the same for a check). Everything the program runs
// in the background goes through it, so a crash leaves a line in the log instead of a tray icon that vanished.
func Go(log func(level, msg string), what string, fn func()) {
	go func() {
		defer func() {
			if p := recover(); p != nil && log != nil {
				log("ERROR", fmt.Sprintf("%s stopped after an error: %v", what, p))
			}
		}()
		fn()
	}()
}
