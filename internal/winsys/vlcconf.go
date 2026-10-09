package winsys

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// VLC's own default port for its web interface.
const vlcDefaultPort = 8080

// VlcrcValue finds an option in vlcrc text. VLC reads the file line by line and ignores the [section] lines, so the
// last "key=value" line wins; commented lines ("#key=" is how VLC writes a default) are skipped.
func VlcrcValue(text, key string) (string, bool) {
	val, found := "", false
	for _, l := range strings.Split(strings.TrimPrefix(text, "\ufeff"), "\n") {
		l = strings.TrimSuffix(l, "\r")
		if k, v, ok := strings.Cut(l, "="); ok && k == key {
			val, found = v, true
		}
	}
	return val, found
}

// SetVlcrcValue sets key=value in vlcrc text: on its own line if there is one, else right after VLC's commented
// default ("#key="), else at the top of [section] (added at the end when missing). A byte-order mark and the line
// endings are kept.
func SetVlcrcValue(text, section, key, value string) string {
	bom := ""
	if strings.HasPrefix(text, "\ufeff") {
		bom, text = "\ufeff", text[len("\ufeff"):]
	}
	eol := "\n" // what VLC writes, also on Windows
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	line := key + "=" + value
	set, comment, header := false, -1, -1
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, key+"="):
			lines[i], set = line, true
		case strings.HasPrefix(l, "#"+key+"=") && comment < 0:
			comment = i
		case header < 0 && (l == "["+section+"]" || strings.HasPrefix(l, "["+section+"] ")):
			header = i
		}
	}
	insert := func(at int) { lines = append(lines[:at], append([]string{line}, lines[at:]...)...) }
	switch {
	case set:
	case comment >= 0:
		insert(comment + 1)
	case header >= 0:
		insert(header + 1)
	default:
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = lines[:n-1]
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "["+section+"]", line, "")
	}
	return bom + strings.Join(lines, eol)
}

// vlcHTTPOn reports whether an extraintf value includes the web interface.
func vlcHTTPOn(extraintf string) bool {
	for _, m := range strings.FieldsFunc(extraintf, func(r rune) bool { return r == ':' || r == ',' }) {
		if m = strings.TrimSpace(m); m == "http" || m == "luahttp" {
			return true
		}
	}
	return false
}

// vlcReady reports whether vlcrc already has the web interface on with a password (VLC keeps it closed without one).
func vlcReady(text string) bool {
	intf, _ := VlcrcValue(text, "extraintf")
	pw, _ := VlcrcValue(text, "http-password")
	return vlcHTTPOn(intf) && pw != ""
}

// vlcConfPaths lists where VLC reads vlcrc, most specific first: the "portable" folder next to a known vlc.exe (VLC
// uses it instead of %APPDATA%\vlc when it exists), then the user's VLC folder. present tells whether VLC seems to
// be on this PC at all.
func vlcConfPaths(exeDirs []string) (paths []string, present bool) {
	if p, err := exec.LookPath("vlc"); err == nil {
		exeDirs = append(exeDirs, filepath.Dir(p))
	}
	for _, pf := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if d := filepath.Join(pf, "VideoLAN", "VLC"); pf != "" && dirExists(d) {
			exeDirs = append(exeDirs, d)
		}
	}
	for _, d := range exeDirs {
		present = true
		if dirExists(filepath.Join(d, "portable")) {
			paths = append(paths, filepath.Join(d, "portable", "vlcrc"))
		}
	}
	if ad := os.Getenv("APPDATA"); ad != "" {
		dir := filepath.Join(ad, "vlc")
		if dirExists(dir) {
			present = true
		}
		paths = append(paths, filepath.Join(dir, "vlcrc"))
	}
	return paths, present
}

// vlcResult is what setting up VLC did.
type vlcResult struct {
	Message  string // empty when VLC is not on this PC
	Port     int    // the port and password VLC's web interface uses (0 / "" when not set up)
	Password string
	Edited   string // the file that was changed
	OK       bool
}

// enableVlcHTTP switches VLC's web interface on in vlcrc (file; text is its content): extraintf gets "http", and a
// password (VLC keeps the interface closed without one). A password and port the user already set are kept and
// reported back; otherwise password (or a new random one) and port are used. Switched on by us, the interface only
// answers this PC (http-host), unless the user chose a host. VLC must not be running: it rewrites vlcrc when it exits.
func enableVlcHTTP(file, text string, port int, password string) vlcResult {
	cur := func(k string) string { v, _ := VlcrcValue(text, k); return v }
	usePort := port
	if p, err := strconv.Atoi(strings.TrimSpace(cur("http-port"))); err == nil && p > 0 && p < 65536 {
		usePort = p
	}
	if usePort <= 0 {
		usePort = vlcDefaultPort
	}
	if vlcReady(text) {
		return vlcResult{Message: "VLC's web interface was already on (port " + strconv.Itoa(usePort) + ").", Port: usePort, Password: cur("http-password"), OK: true}
	}
	t := text
	wasOn := vlcHTTPOn(cur("extraintf"))
	if !wasOn {
		intf := strings.TrimSpace(cur("extraintf"))
		if intf != "" {
			intf += ":"
		}
		t = SetVlcrcValue(t, "core", "extraintf", intf+"http")
	}
	pw := cur("http-password")
	if pw == "" {
		pw = password
	}
	if pw == "" {
		b := make([]byte, 12)
		if _, err := rand.Read(b); err != nil {
			return vlcResult{Message: "VLC: could not make a password."}
		}
		pw = hex.EncodeToString(b)
	}
	t = SetVlcrcValue(t, "lua", "http-password", pw)
	local := false
	if _, set := VlcrcValue(t, "http-host"); !wasOn && !set {
		t = SetVlcrcValue(t, "core", "http-host", "127.0.0.1")
		local = true
	}
	if _, set := VlcrcValue(t, "http-port"); !set && usePort != vlcDefaultPort {
		t = SetVlcrcValue(t, "core", "http-port", strconv.Itoa(usePort))
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return vlcResult{Message: "VLC: could not create " + filepath.Dir(file) + "."}
	}
	if err := os.WriteFile(file, []byte(t), 0o644); err != nil {
		return vlcResult{Message: "VLC: could not write " + file + "."}
	}
	where := "port " + strconv.Itoa(usePort)
	if local {
		where += ", for this PC only"
	}
	return vlcResult{Message: "VLC web interface switched on (" + where + ").", Port: usePort, Password: pw, Edited: file, OK: true}
}
