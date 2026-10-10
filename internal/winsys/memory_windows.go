package winsys

import "runtime/debug"

var pSetProcessWorkingSetSize = kernel32.NewProc("SetProcessWorkingSetSize")

// TrimMemory hands unused memory back to the system: the Go heap's free pages, then the pages the program is not
// using right now (Windows brings them back from memory when they are needed again). Called when presence starts
// resting.
func TrimMemory() {
	debug.FreeOSMemory()
	const current = ^uintptr(0) // GetCurrentProcess()
	pSetProcessWorkingSetSize.Call(current, ^uintptr(0), ^uintptr(0))
}
