package store

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	crypt32                = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree          = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")
)

// dataBlob is DATA_BLOB.
type dataBlob struct {
	size uint32
	data *byte
}

// cryptProtectUIForbidden (CRYPTPROTECT_UI_FORBIDDEN): never show a prompt; fail instead.
const cryptProtectUIForbidden = 0x1

// dpapi is Windows' per-user data protection: what it encrypts only the same user on the same computer can decrypt.
type dpapi struct{}

func systemSealer() sealer { return dpapi{} }

func (dpapi) seal(plain []byte) ([]byte, error) { return dpapiCall(procCryptProtectData, plain) }
func (dpapi) open(sealed []byte) ([]byte, error) {
	return dpapiCall(procCryptUnprotectData, sealed)
}

// dpapiCall runs CryptProtectData or CryptUnprotectData (both take the same arguments here: the input, no description,
// no extra entropy, no prompt) and copies the output out of the memory Windows allocated for it, which it then frees.
func dpapiCall(proc *syscall.LazyProc, in []byte) ([]byte, error) {
	buf := make([]byte, len(in)+1) // one byte more, so that even empty input has an address
	copy(buf, in)
	inBlob := dataBlob{size: uint32(len(in)), data: &buf[0]}
	var out dataBlob
	r, _, err := proc.Call(uintptr(unsafe.Pointer(&inBlob)), 0, 0, 0, 0, cryptProtectUIForbidden, uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(buf)
	if r == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	return append([]byte(nil), unsafe.Slice(out.data, out.size)...), nil
}
