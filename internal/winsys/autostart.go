package winsys

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
)

// runNameBase is the name of the registry value that starts the program at login.
const runNameBase = "MPCvibedRPC"

// runName is the registry value for run-at-login. A copy that uses a data folder of its own (MPCRPC_HOME: a portable
// setup, a test, a development build) gets a value of its own, derived from that folder, so it can never switch the
// installed program's run-at-login on or off.
func runName() string { return runNameFor(os.Getenv("MPCRPC_HOME")) }

func runNameFor(home string) string {
	home = strings.ToLower(strings.TrimRight(strings.TrimSpace(home), `\/`))
	if home == "" {
		return runNameBase
	}
	sum := sha256.Sum256([]byte(home))
	return runNameBase + "-" + hex.EncodeToString(sum[:4])
}
