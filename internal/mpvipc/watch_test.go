package mpvipc

import (
	"bufio"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A line longer than the read buffer is skipped up to its newline, and the events after it still count.
func TestWatchSkipsALongLine(t *testing.T) {
	player, ours := net.Pipe()
	var changes atomic.Int32
	done := make(chan error, 1)
	go func() { done <- Watch(ours, func() { changes.Add(1) }) }()
	r := bufio.NewReader(player)
	if _, err := r.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	go func() {
		long := `{"event":"seek","pad":"` + strings.Repeat("x", 200*1024) + `"}` + "\n"
		player.Write([]byte(long))
		player.Write([]byte(`{"event":"seek"}` + "\n"))
		player.Close()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not return after the player closed")
	}
	if n := changes.Load(); n != 1 {
		t.Fatalf("%d changes reported; want 1 (the long line is skipped, the seek after it counts)", n)
	}
}

// Watch reports pauses, seeks and new files, but not the first report of the observed property, and returns when
// the connection closes.
func TestWatch(t *testing.T) {
	player, ours := net.Pipe()
	var changes atomic.Int32
	done := make(chan error, 1)
	go func() { done <- Watch(ours, func() { changes.Add(1) }) }()

	r := bufio.NewReader(player)
	line, err := r.ReadString('\n')
	if err != nil || !strings.Contains(line, `"observe_property",1,"pause"`) {
		t.Fatalf("Watch asked %q (%v)", line, err)
	}
	for _, ev := range []string{
		`{"event":"property-change","id":1,"name":"pause","data":false}`, // the current value: not a change
		`{"request_id":100,"error":"success"}`,
		`{"event":"audio-reconfig"}`,
		`{"event":"property-change","id":1,"name":"pause","data":true}`, // 1
		`{"event":"seek"}`,                    // 2
		`{"event":"playback-restart"}`,        // 3
		`{"event":"end-file","reason":"eof"}`, // 4
		`{"event":"file-loaded"}`,             // 5
		`not json`,
	} {
		if _, err := player.Write([]byte(ev + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	player.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after the player closed")
	}
	if n := changes.Load(); n != 5 {
		t.Fatalf("%d changes reported; want 5", n)
	}
}
