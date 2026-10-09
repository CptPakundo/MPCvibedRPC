// Package pipe connects to local IPC endpoints: named pipes on Windows, Unix sockets elsewhere. Discord, mpv and
// MPC-QT all listen on one.
package pipe

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Path turns an endpoint name into what Dial expects. A bare name such as "mpvsocket" becomes \\.\pipe\mpvsocket on
// Windows and a socket in the temporary folder elsewhere (where mpv's examples and MPC-QT put theirs); a full path
// is used as it is.
func Path(name string) string {
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(name, `\\`) {
			return name
		}
		return `\\.\pipe\` + name
	}
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(os.TempDir(), name)
}
