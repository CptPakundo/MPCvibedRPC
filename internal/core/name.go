package core

import "os"

// Name is the program's name: its data folder, run-at-login entry, window identity and the answer of its local API
// ("app" in /api/ping) carry it.
const Name = "MPRP"

// FormerName is the name the program had before it was renamed. Only the steps that carry an existing install over
// use it: the data folder, the run-at-login entry and the check for a copy of the former version that still runs.
const FormerName = "MPCvibedRPC"

// FormerEnv pairs each environment variable the program reads with the one the former version read for the same
// thing ({current, former}). The former version starts the copy it updated itself to with its own environment: a copy
// with a data folder of its own passes it as MPCRPC_HOME, and the new copy must use that folder too. The env lines of
// tools/ci/former-names.txt list the same pairs (TestFormerEnvMatchesTheList).
var FormerEnv = [][2]string{
	{"MPRP_HOME", "MPCRPC_HOME"},
	{"MPRP_BROWSER", "MPCRPC_BROWSER"},
	{"MPRP_FOREGROUND", "MPCRPC_FOREGROUND"},
	{"MPRP_UPDATE_API", "MPCRPC_UPDATE_API"},
}

// AdoptFormerEnv gives each variable of FormerEnv that is not set the value of its former twin, when that one is set.
// main calls it first, before anything reads the environment.
func AdoptFormerEnv() {
	for _, p := range FormerEnv {
		if _, set := os.LookupEnv(p[0]); set {
			continue
		}
		if v, set := os.LookupEnv(p[1]); set {
			_ = os.Setenv(p[0], v)
		}
	}
}
