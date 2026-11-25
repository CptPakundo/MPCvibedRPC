package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The env lines of tools/ci/former-names.txt (which the release check reads) are FormerEnv, in the same order.
func TestFormerEnvMatchesTheList(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "tools", "ci", "former-names.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var listed [][2]string
	for _, line := range strings.Split(string(b), "\n") {
		if w := strings.Fields(line); len(w) == 3 && w[0] == "env" {
			listed = append(listed, [2]string{w[1], w[2]})
		}
	}
	if !reflect.DeepEqual(listed, FormerEnv) {
		t.Errorf("former-names.txt lists %v, FormerEnv is %v", listed, FormerEnv)
	}
}

// unsetEnv removes a variable for the rest of the test (and puts it back afterwards).
func unsetEnv(t *testing.T, k string) {
	t.Setenv(k, "")
	os.Unsetenv(k)
}

func TestAdoptFormerEnv(t *testing.T) {
	for _, p := range FormerEnv {
		unsetEnv(t, p[0])
		unsetEnv(t, p[1])
	}
	t.Setenv("MPCRPC_HOME", "/tmp/sample-home")
	t.Setenv("MPCRPC_BROWSER", "none")
	t.Setenv("MPRP_BROWSER", "") // set, even if empty: the current name wins
	AdoptFormerEnv()
	if v, ok := os.LookupEnv("MPRP_HOME"); !ok || v != "/tmp/sample-home" {
		t.Errorf("MPRP_HOME = %q (%v), want the former variable's value", v, ok)
	}
	if v := os.Getenv("MPRP_BROWSER"); v != "" {
		t.Errorf("MPRP_BROWSER = %q: a variable that is set must be left as it is", v)
	}
	if _, ok := os.LookupEnv("MPRP_UPDATE_API"); ok {
		t.Error("MPRP_UPDATE_API was set although neither name was")
	}
}
