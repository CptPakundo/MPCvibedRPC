// mkrsrc writes a Windows resource object (.syso) holding the program's icon, version information and application manifest (per-monitor DPI awareness), so the
// finished MPCvibedRPC.exe has an icon in Explorer and proper details in Task Manager. The Go linker picks the
// .syso up automatically when it sits next to the main package. Standard library only.
//
//	go run ./tools/mkrsrc -icon internal/assets/icon.ico -version 0.9.9 -out cmd/mpcvibedrpc/rsrc_windows_amd64.syso
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"unicode/utf16"
)

var le = binary.LittleEndian

func main() {
	icon := flag.String("icon", "internal/assets/icon.ico", "icon file")
	version := flag.String("version", "0.0.0", "version, e.g. 0.9.9")
	out := flag.String("out", "rsrc_windows_amd64.syso", "output object file")
	flag.Parse()
	ico, err := os.ReadFile(*icon)
	if err != nil {
		fatal(err)
	}
	obj, err := Build(ico, *version)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, obj, 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "mkrsrc:", err); os.Exit(1) }

// Resource type ids.
const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
	rtManifest  = 24
)

type leaf struct {
	typ, name, lang uint32
	data            []byte
}

// Build makes the COFF object for an .ico file and a version string.
func Build(ico []byte, version string) ([]byte, error) {
	if len(ico) < 6 || le.Uint16(ico[0:]) != 0 || le.Uint16(ico[2:]) != 1 {
		return nil, fmt.Errorf("not an .ico file")
	}
	n := int(le.Uint16(ico[4:]))
	if n == 0 || len(ico) < 6+16*n {
		return nil, fmt.Errorf("bad .ico directory")
	}
	var leaves []leaf
	group := new(bytes.Buffer)
	binary.Write(group, le, [3]uint16{0, 1, uint16(n)})
	for i := 0; i < n; i++ {
		e := ico[6+16*i:]
		size, off := le.Uint32(e[8:]), le.Uint32(e[12:])
		if int(off)+int(size) > len(ico) {
			return nil, fmt.Errorf("icon image %d is cut off", i)
		}
		id := uint16(i + 1)
		leaves = append(leaves, leaf{rtIcon, uint32(id), 0x0409, ico[off : off+size]})
		group.Write(e[0:8]) // width, height, colors, reserved, planes, bit count
		binary.Write(group, le, size)
		binary.Write(group, le, id)
	}
	leaves = append(leaves, leaf{rtGroupIcon, 1, 0x0409, group.Bytes()})
	vi, err := versionInfo(version)
	if err != nil {
		return nil, err
	}
	leaves = append(leaves, leaf{rtVersion, 1, 0x0409, vi})
	leaves = append(leaves, leaf{rtManifest, 1, 0x0409, []byte(manifest)})
	return coff(leaves), nil
}

// manifest makes the process DPI-aware: without it Windows scales the tray icon and menu as bitmaps on high-DPI screens.
const manifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2,PerMonitor</dpiAwareness>
    </windowsSettings>
  </application>
</assembly>
`

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

// ---- resource tree -----------------------------------------------------------------

func coff(leaves []leaf) []byte {
	// group by type, then by name
	types := []uint32{}
	byType := map[uint32][]leaf{}
	for _, l := range leaves {
		if _, ok := byType[l.typ]; !ok {
			types = append(types, l.typ)
		}
		byType[l.typ] = append(byType[l.typ], l)
	}
	sortU32(types)

	// layout: root dir | per-type dirs | per-name dirs | data entries | data
	rootSize := 16 + 8*len(types)
	off := rootSize
	typeDirOff := map[uint32]int{}
	for _, t := range types {
		typeDirOff[t] = off
		off += 16 + 8*len(byType[t])
	}
	nameDirOff := map[[2]uint32]int{}
	for _, t := range types {
		for _, l := range byType[t] {
			nameDirOff[[2]uint32{t, l.name}] = off
			off += 16 + 8 // one language per name
		}
	}
	entryOff := map[[2]uint32]int{}
	for _, t := range types {
		for _, l := range byType[t] {
			entryOff[[2]uint32{t, l.name}] = off
			off += 16
		}
	}
	dataOff := map[[2]uint32]int{}
	cur := off
	for _, t := range types {
		for _, l := range byType[t] {
			dataOff[[2]uint32{t, l.name}] = cur
			cur += (len(l.data) + 3) &^ 3
		}
	}

	raw := new(bytes.Buffer)
	dir := func(ids int) { binary.Write(raw, le, [4]uint32{0, 0, 0, uint32(ids) << 16}) } // id entries in the high half of the last dword
	dir(len(types))
	for _, t := range types {
		binary.Write(raw, le, [2]uint32{t, 0x80000000 | uint32(typeDirOff[t])})
	}
	for _, t := range types {
		dir(len(byType[t]))
		ls := byType[t]
		sortLeaves(ls)
		for _, l := range ls {
			binary.Write(raw, le, [2]uint32{l.name, 0x80000000 | uint32(nameDirOff[[2]uint32{t, l.name}])})
		}
	}
	for _, t := range types {
		for _, l := range byType[t] {
			dir(1)
			binary.Write(raw, le, [2]uint32{l.lang, uint32(entryOff[[2]uint32{t, l.name}])})
		}
	}
	var relocAt []uint32
	for _, t := range types {
		for _, l := range byType[t] {
			relocAt = append(relocAt, uint32(raw.Len()))
			binary.Write(raw, le, [4]uint32{uint32(dataOff[[2]uint32{t, l.name}]), uint32(len(l.data)), 0, 0}) // RVA fixed by relocation
		}
	}
	for _, t := range types {
		for _, l := range byType[t] {
			raw.Write(l.data)
			pad4(raw)
		}
	}

	// object file
	const hdr, shdr = 20, 40
	rawOff := hdr + shdr
	relOff := rawOff + raw.Len()
	symOff := relOff + 10*len(relocAt)
	out := new(bytes.Buffer)
	binary.Write(out, le, struct {
		Machine, Sections  uint16
		Time, SymPtr, Syms uint32
		OptSize, Chars     uint16
	}{0x8664, 1, 0, uint32(symOff), 2, 0, 0})
	var name [8]byte
	copy(name[:], ".rsrc")
	out.Write(name[:])
	binary.Write(out, le, struct {
		VSize, VAddr, RawSize, RawPtr, RelPtr, LinePtr uint32
		Relocs, Lines                                  uint16
		Chars                                          uint32
	}{0, 0, uint32(raw.Len()), uint32(rawOff), uint32(relOff), 0, uint16(len(relocAt)), 0, 0xC0000040})
	out.Write(raw.Bytes())
	for _, at := range relocAt {
		binary.Write(out, le, struct {
			Addr, Sym uint32
			Type      uint16
		}{at, 0, 3}) // IMAGE_REL_AMD64_ADDR32NB against the section symbol
	}
	// symbol 0: the section; symbol 1: its auxiliary record
	out.Write(name[:])
	binary.Write(out, le, struct {
		Value   uint32
		Section int16
		Type    uint16
		Class   uint8
		Aux     uint8
	}{0, 1, 0, 3, 1})
	binary.Write(out, le, struct {
		Len           uint32
		Relocs, Lines uint16
		Check         uint32
		Number        uint16
		Selection     uint8
		Unused        [3]byte
	}{uint32(raw.Len()), uint16(len(relocAt)), 0, 0, 0, 0, [3]byte{}})
	binary.Write(out, le, uint32(4)) // empty string table
	return out.Bytes()
}

func sortU32(a []uint32) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func sortLeaves(a []leaf) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j].name < a[j-1].name; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

// ---- VERSIONINFO -------------------------------------------------------------------

func u16z(s string) []byte {
	u := utf16.Encode([]rune(s))
	u = append(u, 0)
	b := make([]byte, 2*len(u))
	for i, c := range u {
		le.PutUint16(b[2*i:], c)
	}
	return b
}

// block writes one version-info structure: length, value length, type, key, padding, value, then children.
func block(key string, valueLen, typ uint16, value []byte, children ...[]byte) []byte {
	b := new(bytes.Buffer)
	b.Write([]byte{0, 0}) // length, patched below
	binary.Write(b, le, valueLen)
	binary.Write(b, le, typ)
	b.Write(u16z(key))
	pad4(b)
	b.Write(value)
	for _, c := range children {
		pad4(b)
		b.Write(c)
	}
	out := b.Bytes()
	le.PutUint16(out[0:], uint16(len(out)))
	return out
}

var reVer = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)`)

func versionInfo(version string) ([]byte, error) {
	m := reVer.FindStringSubmatch(version)
	if m == nil {
		return nil, fmt.Errorf("version %q must start with major.minor.patch", version)
	}
	var v [4]uint16
	for i := 0; i < 3; i++ {
		n, _ := strconv.Atoi(m[i+1])
		v[i] = uint16(n)
	}
	ms := uint32(v[0])<<16 | uint32(v[1])
	ls := uint32(v[2])<<16 | uint32(v[3])
	fixed := new(bytes.Buffer)
	binary.Write(fixed, le, [13]uint32{0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x00040004, 1, 0, 0, 0})

	str := func(k, v string) []byte {
		val := u16z(v)
		return block(k, uint16(len(val)/2), 1, val)
	}
	var strs [][]byte
	for _, kv := range [][2]string{
		{"CompanyName", ""},
		{"Comments", "Entirely AI-generated (vibe coded); no human authorship is claimed. No warranty."},
		{"FileDescription", "MPCvibedRPC - Discord Rich Presence for MPC-HC, MPC-BE, MPC-QT, mpv and VLC"},
		{"FileVersion", version},
		{"InternalName", "MPCvibedRPC"},
		{"LegalCopyright", "MIT License"},
		{"OriginalFilename", "MPCvibedRPC.exe"},
		{"ProductName", "MPCvibedRPC"},
		{"ProductVersion", version},
	} {
		strs = append(strs, str(kv[0], kv[1]))
	}
	table := block("040904B0", 0, 1, nil, strs...)
	sfi := block("StringFileInfo", 0, 1, nil, table)
	tr := make([]byte, 4)
	le.PutUint32(tr, 0x04B00409)
	vfi := block("VarFileInfo", 0, 1, nil, block("Translation", 4, 0, tr))
	return block("VS_VERSION_INFO", 52, 0, fixed.Bytes(), sfi, vfi), nil
}
