package dbus

import (
	"encoding/binary"
	"testing"
	"time"
)

// An array of dictionary entries with no members ("a{}") takes no bytes per entry, so a message that claims some
// could keep the decoder busy for ever: it must give up instead, like the plain array branch does.
func TestEmptyDictEntryEndsEarly(t *testing.T) {
	d := &decoder{b: []byte{8, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}, order: binary.LittleEndian}
	done := make(chan error, 1)
	go func() { _, err := d.value("a{}"); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a dictionary entry of no size should be an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the decoder loops on a dictionary entry of no size")
	}
}
