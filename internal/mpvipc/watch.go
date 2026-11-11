package mpvipc

import (
	"bufio"
	"encoding/json"
	"io"
)

// watchEvents are the events after which what the player shows has changed: a new file, the end of one, a seek.
var watchEvents = map[string]bool{"file-loaded": true, "end-file": true, "seek": true, "playback-restart": true}

// Watch keeps conn open and calls onChange whenever the player pauses, resumes, seeks or changes file, until conn
// closes (the player quit, or the caller closed it). It reads nothing else: what plays is still read with Query.
func Watch(conn io.ReadWriteCloser, onChange func()) error {
	defer conn.Close()
	// mpv answers an observed property at once with its current value: that first report is not a change
	if _, err := io.WriteString(conn, `{"command":["observe_property",1,"pause"],"request_id":100}`+"\n"); err != nil {
		return err
	}
	r := bufio.NewReaderSize(conn, 64*1024)
	seenPause := false
	for {
		// ReadSlice never stores more than the buffer: a longer line (no event of ours is) is skipped up to its end
		line, err := r.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			for err == bufio.ErrBufferFull {
				_, err = r.ReadSlice('\n')
			}
			if err != nil {
				return err
			}
			continue
		}
		if len(line) > 0 {
			var ev struct {
				Event string `json:"event"`
				Name  string `json:"name"`
			}
			if json.Unmarshal(line, &ev) == nil {
				switch {
				case ev.Event == "property-change" && ev.Name == "pause":
					if seenPause {
						onChange()
					}
					seenPause = true
				case watchEvents[ev.Event]:
					onChange()
				}
			}
		}
		if err != nil {
			return err
		}
	}
}
