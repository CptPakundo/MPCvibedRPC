package store

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// fakeSealer stands in for the system's encryption: it reverses the bytes behind a marker, and cannot open what it did
// not seal (as DPAPI cannot open another user's values).
type fakeSealer struct{ fail bool }

func (f fakeSealer) seal(p []byte) ([]byte, error) {
	if f.fail {
		return nil, errors.New("sealing is off")
	}
	b := []byte("sealed:")
	for i := len(p) - 1; i >= 0; i-- {
		b = append(b, p[i])
	}
	return b, nil
}

func (fakeSealer) open(s []byte) ([]byte, error) {
	rest, ok := strings.CutPrefix(string(s), "sealed:")
	if !ok {
		return nil, errors.New("not ours")
	}
	b := []byte(rest)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b, nil
}

// sealedStore is a store in dir with the fake encryption, and the log lines it writes.
func sealedStore(t *testing.T, dir string, f fakeSealer) (*Store, *[]string) {
	t.Helper()
	s := openStore(t, dir)
	s.sealer = f
	var logs []string
	s.Log = func(level, msg string) { logs = append(logs, level+" "+msg) }
	return s, &logs
}

func TestSecretKeysAreTheSignIns(t *testing.T) {
	if want := []string{"plexToken", "plexKey", "stremioKey", "jellyfinToken"}; !reflect.DeepEqual(secretKeys, want) {
		t.Errorf("secretKeys = %v, want %v", secretKeys, want)
	}
}

func TestSecretsAreSavedEncrypted(t *testing.T) {
	s, logs := sealedStore(t, t.TempDir(), fakeSealer{})
	keys := []string{"plexToken", "plexKey", "stremioKey", "jellyfinToken", "plexUser"}
	vals := map[string]any{"plexToken": "sample-token", "plexKey": "sample-key", "stremioKey": "stremio-key", "jellyfinToken": "jf-token", "plexUser": "SampleUser"}
	if err := s.SetHidden(keys, vals); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.File)
	for _, v := range []string{"sample-token", "sample-key", "stremio-key", "jf-token"} {
		if strings.Contains(string(b), v) {
			t.Errorf("%q is in config.json as plain text:\n%s", v, b)
		}
	}
	if !strings.Contains(string(b), `"plexToken": "dpapi:`) || !strings.Contains(string(b), `"plexUser": "SampleUser"`) {
		t.Errorf("only the secrets are encrypted:\n%s", b)
	}
	c := s.Config()
	if c.PlexToken != "sample-token" || c.PlexKey != "sample-key" || c.StremioKey != "stremio-key" || c.JellyfinToken != "jf-token" {
		t.Errorf("read back %+v", c.Secrets)
	}
	// another store (the next start) decrypts them too
	if c, _ := sealedStore(t, s.Dir, fakeSealer{}); c.Config().PlexToken != "sample-token" {
		t.Error("a new start reads the encrypted token")
	}
	// a write that does not change a secret keeps its encrypted text
	if err := s.SetSettings(map[string]any{"pollInterval": 2000}); err != nil {
		t.Fatal(err)
	}
	if b2, _ := os.ReadFile(s.File); !strings.Contains(string(b2), `"pollInterval": 2000`) || sealedValue(t, b2, "plexToken") != sealedValue(t, b, "plexToken") {
		t.Errorf("the token's encrypted text changed:\n%s\n%s", b, b2)
	}
	for k := range s.Values() {
		for _, sk := range secretKeys {
			if k == sk {
				t.Errorf("the window receives %s", k)
			}
		}
	}
	if len(*logs) != 0 {
		t.Errorf("log %q", *logs)
	}
}

// sealedValue is the encrypted text config.json holds for key.
func sealedValue(t *testing.T, file []byte, key string) string {
	t.Helper()
	_, rest, ok := strings.Cut(string(file), `"`+key+`": "`)
	if !ok {
		t.Fatalf("no %s in %s", key, file)
	}
	v, _, _ := strings.Cut(rest, `"`)
	return v
}

func TestPlainSecretsLoadAndAreEncryptedAtTheNextWrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/config.json", []byte(`{"stremioKey": "older-key", "vlcPassword": "typed-pass", "tmdbApiKey": "typed-key"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := sealedStore(t, dir, fakeSealer{})
	if c := s.Config(); c.StremioKey != "older-key" || c.VlcPassword != "typed-pass" {
		t.Fatalf("a plain value from an older version loads: %+v", c)
	}
	if err := s.SetSettings(map[string]any{"pollInterval": 2000}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.File)
	if strings.Contains(string(b), "older-key") || !strings.Contains(string(b), `"stremioKey": "dpapi:`) {
		t.Errorf("the next write encrypts the sign-in:\n%s", b)
	}
	if !strings.Contains(string(b), `"vlcPassword": "typed-pass"`) || !strings.Contains(string(b), `"tmdbApiKey": "typed-key"`) {
		t.Errorf("the secrets typed in the window stay plain text:\n%s", b)
	}
	if s.Config().StremioKey != "older-key" {
		t.Error("read back after encrypting")
	}
}

func TestUndecryptableSecretReadsAsSignedOut(t *testing.T) {
	dir := t.TempDir()
	// encrypted by another user or computer: the fake cannot open it
	if err := os.WriteFile(dir+"/config.json", []byte(`{"plexToken": "dpapi:b3RoZXI=", "plexUser": "SampleUser", "jellyfinToken": "dpapi:%%%", "pollInterval": 3000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, logs := sealedStore(t, dir, fakeSealer{})
	for i := 0; i < 3; i++ {
		if c := s.Config(); c.PlexToken != "" || c.JellyfinToken != "" || c.PlexUser != "SampleUser" || c.PollInterval != 3000 {
			t.Fatalf("read %+v", c)
		}
	}
	if len(*logs) != 2 || !strings.Contains((*logs)[0], "plexToken") || !strings.Contains((*logs)[1], "jellyfinToken") || strings.Contains(strings.Join(*logs, ""), "b3RoZXI") {
		t.Errorf("one line per value, without it: %q", *logs)
	}
	// with no encryption on this system (macOS, Linux) an encrypted value cannot be read either
	n := openStore(t, dir)
	n.sealer = nil
	if n.Config().PlexToken != "" {
		t.Error("no encryption: the encrypted value reads as signed out")
	}

	// a save of something else keeps the encrypted text in the file (with and without encryption here)
	for _, st := range []*Store{s, n} {
		if err := st.SetSettings(map[string]any{"pollInterval": 4000}); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(st.File)
		if !strings.Contains(string(b), `"plexToken": "dpapi:b3RoZXI="`) || !strings.Contains(string(b), `"jellyfinToken": "dpapi:%%%"`) {
			t.Errorf("a save of another setting dropped the undecryptable values:\n%s", b)
		}
		if c := st.Config(); c.PlexToken != "" || c.JellyfinToken != "" || c.PollInterval != 4000 {
			t.Errorf("after the save: %+v", c)
		}
	}
	// a new sign-in replaces it; a sign-out removes it
	if err := s.SetHidden([]string{"plexToken", "jellyfinToken"}, map[string]any{"plexToken": "sample-token"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.File)
	if strings.Contains(string(b), "b3RoZXI") || strings.Contains(string(b), "jellyfinToken") || s.Config().PlexToken != "sample-token" {
		t.Errorf("after a sign-in and a sign-out:\n%s", b)
	}
	if err := s.SetSettings(map[string]any{"pollInterval": 5000}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.File); strings.Contains(string(b), "jellyfinToken") || strings.Contains(string(b), "b3RoZXI") {
		t.Errorf("a value replaced or removed came back:\n%s", b)
	}
}

func TestSecretsStayPlainWithoutEncryption(t *testing.T) {
	s := openStore(t, t.TempDir())
	s.sealer = nil // macOS and Linux
	if err := s.SetHidden([]string{"plexToken"}, map[string]any{"plexToken": "sample-token"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.File); !strings.Contains(string(b), `"plexToken": "sample-token"`) {
		t.Errorf("plain text as before:\n%s", b)
	}
}

func TestFailedEncryptionSavesPlainAndLogsOnce(t *testing.T) {
	s, logs := sealedStore(t, t.TempDir(), fakeSealer{fail: true})
	for i := 0; i < 2; i++ {
		if err := s.SetHidden([]string{"plexToken"}, map[string]any{"plexToken": "sample-token"}); err != nil {
			t.Fatal(err)
		}
	}
	if s.Config().PlexToken != "sample-token" {
		t.Error("the sign-in is kept")
	}
	if len(*logs) != 1 || strings.Contains((*logs)[0], "sample-token") {
		t.Errorf("one log line, without the value: %q", *logs)
	}
}
