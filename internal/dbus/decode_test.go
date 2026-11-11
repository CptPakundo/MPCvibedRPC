package dbus

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// A header that declares a body over the cap is refused from the header alone: the reader holds no body, so an
// attempt to read one would fail with a different error.
func TestOversizedMessageRefusedUnread(t *testing.T) {
	hdr := make([]byte, 16)
	hdr[0], hdr[1], hdr[3] = 'l', 2, 1
	binary.LittleEndian.PutUint32(hdr[4:], maxMessage+1) // body length
	_, err := ReadMessage(bytes.NewReader(hdr))
	if err == nil || err.Error() != "dbus: message too large" {
		t.Fatalf("got %v, want the size refusal", err)
	}
}

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
