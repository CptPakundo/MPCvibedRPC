//go:build !windows

package pipe

import "testing"

func listenPipe(t *testing.T, path string) func() {
	t.Fatal("named pipes are Windows only")
	return nil
}
