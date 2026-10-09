//go:build !windows

package winsys

import (
	"reflect"
	"testing"
)

func TestProfilePids(t *testing.T) {
	ps := `    1 /sbin/init
  200 /opt/google/chrome/chrome --app=http://127.0.0.1:47654/?t=x --user-data-dir=/home/someone/.local/share/MPCvibedRPC/window --no-first-run
  201 /opt/google/chrome/chrome --type=renderer --user-data-dir=/home/someone/.local/share/MPCvibedRPC/window --lang=en
  202 /opt/google/chrome/chrome --user-data-dir=/home/someone/.local/share/MPCvibedRPC/window2
  203 /opt/google/chrome/chrome
  300 /home/someone/MPCvibedRPC --user-data-dir=/home/someone/.local/share/MPCvibedRPC/window
  bad line
`
	got := profilePids(ps, "/home/someone/.local/share/MPCvibedRPC/window", 300)
	if !reflect.DeepEqual(got, []int{200, 201}) {
		t.Errorf("got %v, want [200 201] (not another profile, not the user's browser, not ourselves)", got)
	}
	if got := profilePids(ps, "", 0); got != nil {
		t.Errorf("no profile: %v", got)
	}
}
