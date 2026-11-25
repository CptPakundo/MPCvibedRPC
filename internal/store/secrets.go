package store

import (
	"encoding/base64"
	"reflect"
	"strings"

	"github.com/CptPakundo/MPRP/internal/core"
	"github.com/CptPakundo/MPRP/internal/jsonx"
)

// sealer encrypts the sign-ins' secrets for config.json and decrypts them again. On Windows it is the per-user DPAPI
// (secrets_windows.go): only the same user on the same computer can decrypt. macOS and Linux have none, and keep the
// secrets as plain text.
type sealer interface {
	seal(plain []byte) ([]byte, error)
	open(sealed []byte) ([]byte, error)
}

// sealedPrefix marks an encrypted value in config.json: "dpapi:" and the encrypted bytes in base64. A value without it
// is plain text (written by a version from before the encryption, or on macOS and Linux) and is read as it is.
const sealedPrefix = "dpapi:"

// secretKeys are the settings kept encrypted: the fields of core.Secrets. The secrets typed in the window
// (vlcPassword, kodiPassword, tmdbApiKey) stay plain text: docs/decisions.md says why.
var secretKeys = func() []string {
	var keys []string
	t := reflect.TypeOf(core.Secrets{})
	for i := 0; i < t.NumField(); i++ {
		keys = append(keys, strings.Split(t.Field(i).Tag.Get("json"), ",")[0])
	}
	return keys
}()

// openSecrets decrypts the encrypted secrets of a configuration just read, in place. A value that cannot be decrypted
// (config.json copied from another user or computer, or a system without the encryption) is removed, so that sign-in
// reads as signed out; it is logged once. The caller holds s.mu.
func (s *Store) openSecrets(o *jsonx.Obj) {
	for _, k := range secretKeys {
		v, ok := o.M[k].(string)
		if !ok || !strings.HasPrefix(v, sealedPrefix) {
			continue
		}
		if plain, ok := s.plainOf[v]; ok {
			o.Set(k, plain)
			continue
		}
		plain, err := s.openValue(v)
		if err != nil {
			o.Delete(k)
			if !s.unreadable[v] {
				if s.unreadable == nil {
					s.unreadable = map[string]bool{}
				}
				s.unreadable[v] = true
				s.logf("WARN", "The saved "+k+" cannot be decrypted (the settings file comes from another user or computer), so that sign-in counts as signed out: sign in again.")
			}
			continue
		}
		s.remember(v, plain)
		o.Set(k, plain)
	}
}

// openValue decrypts one "dpapi:" value.
func (s *Store) openValue(v string) (string, error) {
	if s.sealer == nil {
		return "", errNoSealer
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, sealedPrefix))
	if err != nil {
		return "", err
	}
	p, err := s.sealer.open(b)
	if err != nil {
		return "", err
	}
	return string(p), nil
}

// sealSecrets gives the configuration to write with its secrets encrypted (o itself is left as it is). A secret that
// did not change keeps the encrypted text it had, so config.json does not change for nothing. When the encryption
// fails the value is written as plain text, as before the encryption, and that is logged once: a lost sign-in would be
// worse. The caller holds s.mu.
func (s *Store) sealSecrets(o *jsonx.Obj) *jsonx.Obj {
	if s.sealer == nil {
		return o
	}
	var out *jsonx.Obj
	for _, k := range secretKeys {
		v, ok := o.M[k].(string)
		if !ok || v == "" || strings.HasPrefix(v, sealedPrefix) {
			continue
		}
		sealed, ok := s.sealedOf[v]
		if !ok {
			b, err := s.sealer.seal([]byte(v))
			if err != nil {
				if !s.sealFailed {
					s.sealFailed = true
					s.logf("WARN", "The sign-ins could not be encrypted ("+err.Error()+"), so they are saved as plain text.")
				}
				continue
			}
			sealed = sealedPrefix + base64.StdEncoding.EncodeToString(b)
			s.remember(sealed, v)
		}
		if out == nil {
			out = jsonx.NewObj()
			for _, key := range o.Keys {
				out.Set(key, o.M[key])
			}
		}
		out.Set(k, sealed)
	}
	if out == nil {
		return o
	}
	return out
}

// remember keeps a value's encrypted and plain text in memory, so reading it again needs no decryption and writing it
// again keeps the same encrypted text.
func (s *Store) remember(sealed, plain string) {
	if s.plainOf == nil {
		s.plainOf, s.sealedOf = map[string]string{}, map[string]string{}
	}
	s.plainOf[sealed], s.sealedOf[plain] = plain, sealed
}

// logf writes a line to the program's log, when the program gave the store one (Store.Log).
func (s *Store) logf(level, msg string) {
	if s.Log != nil {
		s.Log(level, msg)
	}
}

type sealError string

func (e sealError) Error() string { return string(e) }

// errNoSealer is the error for an encrypted value on a system that has no encryption (a config.json from Windows).
const errNoSealer = sealError("this system keeps no encrypted settings")
