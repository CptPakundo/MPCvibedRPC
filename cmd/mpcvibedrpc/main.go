// MPCvibedRPC: shows what MPC-HC, MPC-BE, MPC-QT, mpv, VLC or PotPlayer (on macOS also IINA; on Linux any video player
// that offers MPRIS), Kodi, or Plex, Jellyfin, Emby or Stremio once signed in, is playing as a Discord Rich Presence. Runs from
// the tray on Windows and in the background on macOS and Linux; the settings are a small local web page opened in an
// app-style window.
//
// Where things are: app.go starts and stops the program, api.go answers the settings window, update.go checks for
// and installs new versions, tray.go is the tray / menu bar icon, accounts.go with plex.go, stremio.go and
// jellyfin.go are the sign-ins, logger.go is the log file.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CptPakundo/MPCvibedRPC/internal/engine"
	"github.com/CptPakundo/MPCvibedRPC/internal/store"
	"github.com/CptPakundo/MPCvibedRPC/internal/updater"
	"github.com/CptPakundo/MPCvibedRPC/internal/winsys"
)

// version is set at build time (-ldflags "-X main.version=...").
var version = "0.9.26"

const preferredPort = 47654

var (
	background  bool
	afterUpdate bool
	serve       bool // macOS: this is the copy that does the work (see detach in launch_darwin.go)
)

func main() {
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--home": // a data folder of its own, as MPCRPC_HOME gives (Windows run-at-login passes it this way)
			if i+1 < len(args) {
				os.Setenv("MPCRPC_HOME", args[i+1])
				i++
			}
		case "--background":
			background = true
		case "--after-update":
			afterUpdate = true
		case "--serve":
			serve = true
		case "--version":
			fmt.Println(version)
			return
		}
	}
	prepareSystem()
	if detach() {
		return
	}
	if v := os.Getenv("MPCRPC_UPDATE_API"); v != "" { // tests and self-hosted mirrors
		updater.APIBase = strings.TrimRight(v, "/")
		updater.WebBase = updater.APIBase
	}
	// on macOS the program runs beside the app's event loop, which needs the main thread (menu bar icon, window)
	winsys.RunMain(func() {
		defer func() {
			if p := recover(); p != nil {
				crash(fmt.Sprint(p))
				os.Exit(1)
			}
		}()
		if err := run(); err != nil {
			crash(err.Error())
			os.Exit(1)
		}
	})
}

func crash(msg string) {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "mpcvibedrpc-crash.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err == nil {
		fmt.Fprintf(f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), msg)
		f.Close()
	}
}

// ipcInfo is ipc.json: how a second copy (and the tests) reach the running one.
type ipcInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	Pid   int    `json:"pid"`
}

// pingExisting finds a running copy of the program (nil when there is none).
func pingExisting(ipcFile string) *ipcInfo {
	raw, err := os.ReadFile(ipcFile)
	if err != nil {
		return nil
	}
	var info ipcInfo
	if json.Unmarshal(raw, &info) != nil || info.Port == 0 {
		return nil
	}
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	res, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", info.Port))
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	var p struct {
		App string `json:"app"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&p) != nil || p.App != "MPCvibedRPC" {
		return nil
	}
	return &info
}

// handOver asks the running copy to show its window (a start in the background just leaves quietly).
func handOver(other *ipcInfo) {
	if background {
		return
	}
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/api/open", other.Port), strings.NewReader("{}"))
	req.Header.Set("X-Token", other.Token)
	c := &http.Client{Timeout: 3 * time.Second}
	if res, err := c.Do(req); err == nil {
		res.Body.Close()
	}
}

func windowURL(port int, token string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/?t=%s", port, token)
}

// applyMu makes "read the saved settings, hand them to the engine" one step: two saves at once (the window and the
// tray, or a sign-in finishing) could otherwise leave the engine on the older of the two.
var applyMu sync.Mutex

// applySettings gives the engine the settings as saved now.
func applySettings(st *store.Store, eng *engine.Engine) {
	applyMu.Lock()
	defer applyMu.Unlock()
	eng.ApplySettings(st.Config())
}

// setHideTitle switches "Hide what I'm watching" from the tray or the menu bar, saved like a switch in the window.
func setHideTitle(st *store.Store, eng *engine.Engine, log func(level, msg string), on bool) {
	if err := st.SetSettings(map[string]any{"hideTitle": on}); err != nil {
		log("ERROR", "Could not save Hide what I'm watching: "+err.Error())
		return
	}
	applySettings(st, eng)
	if on {
		log("INFO", "Hide what I'm watching turned on from the menu.")
	} else {
		log("INFO", "Hide what I'm watching turned off from the menu.")
	}
}
