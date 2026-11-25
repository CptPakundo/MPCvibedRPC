package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/CptPakundo/MPRP/internal/core"
	"github.com/CptPakundo/MPRP/internal/updater"
	"github.com/CptPakundo/MPRP/internal/winsys"
)

// An install made before the program was renamed updates itself in place, so its program file keeps the former
// release name (MPCvibedRPC.exe, MPCvibedRPC-linux-amd64, ...). The program gives such a file the current name
// (MPRP.exe, ...) itself: a file with any other name (a user's own, a development build) is never touched, nor is the
// macOS app (it does not update itself, and its file is inside the app bundle).

// renameMoment is when the program renames its own file.
type renameMoment int

const (
	renameNever         renameMoment = iota // the old version, back after an update that did not start
	renameAtStart                           // a normal start
	renameAfterHandOver                     // after an update, once the copy that started this one can no longer put itself back
)

// whenToRename: never during an update's hand-over (the old copy puts its own file back under the former name if this
// copy does not come up) and never in the copy that came back that way.
func whenToRename(afterUpdate, updateFailed bool) renameMoment {
	switch {
	case updateFailed:
		return renameNever
	case afterUpdate:
		return renameAfterHandOver
	}
	return renameAtStart
}

// formerFileName is this system's former release file name and the current one ("" for both where there is none).
func formerFileName(goos, goarch string) (former, current string) {
	current = updater.AssetFor(goos, goarch)
	if current == "" {
		return "", ""
	}
	return strings.Replace(current, core.Name, core.FormerName, 1), current
}

// renameTarget is the path exe gets: the current name in its folder when exe's file name is exactly the former release
// name (any case on Windows), else "".
func renameTarget(goos, goarch, exe string) string {
	former, current := formerFileName(goos, goarch)
	if former == "" {
		return ""
	}
	base := filepath.Base(exe)
	if base == former || (goos == "windows" && strings.EqualFold(base, former)) {
		return filepath.Join(filepath.Dir(exe), current)
	}
	return ""
}

// renameProgramFile gives exe the current name (renameTarget) and returns the program file's path afterwards (exe when
// it was not renamed) with a line for the log ("": nothing to say). The rule when a file of the current name is there
// already:
//   - the same bytes: exe is a second copy of the same program. It is moved aside as exe + ".old" (deleted now where
//     the system allows it, else at the next start: removeFormerLeftovers) and the program goes on as the other file.
//   - other bytes: both stay as they are, and the start says so in the log.
//
// A folder that cannot be written to (Program Files without the rights) leaves the file as it is, with a warning.
func renameProgramFile(goos, goarch, exe string) (now, level, msg string) {
	to := renameTarget(goos, goarch, exe)
	if to == "" {
		return exe, "", ""
	}
	name := filepath.Base(to)
	if _, err := os.Lstat(to); err == nil {
		if !sameContent(exe, to) {
			return exe, "INFO", "The program file keeps its former name " + filepath.Base(exe) + ": a different " + name + " is already in its folder."
		}
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return exe, "WARN", "The program file " + filepath.Base(exe) + " is a second copy of " + name + " next to it but could not be moved aside: " + err.Error()
		}
		_ = os.Remove(old) // Linux lets a running program's file go; on Windows it goes at the next start
		return to, "INFO", "The program file " + filepath.Base(exe) + " was a second copy of " + name + " next to it, so the program now runs as " + name + "; shortcuts made to the old name need making again."
	}
	if err := renameNoReplace(exe, to); err != nil {
		return exe, "WARN", "Could not rename the program file to " + name + " (" + err.Error() + "); it keeps working under its former name."
	}
	return to, "INFO", "The program file was renamed to " + name + " (the program's new name); shortcuts made to the old name need making again."
}

// sameContent says whether the files a and b hold the same bytes (a program file is a few MB).
func sameContent(a, b string) bool {
	x, err := os.ReadFile(a)
	if err != nil {
		return false
	}
	y, err := os.ReadFile(b)
	return err == nil && bytes.Equal(x, y)
}

// removeFormerLeftovers deletes what an update or renameProgramFile left next to the program file exe under the
// former name (<former>.old, <former>.new), once the program runs under the current name and no file of the former
// name is there any more (one that is there may be another copy, whose update they belong to).
func removeFormerLeftovers(goos, goarch, exe string) {
	former, current := formerFileName(goos, goarch)
	if former == "" || !strings.EqualFold(filepath.Base(exe), current) {
		return
	}
	f := filepath.Join(filepath.Dir(exe), former)
	if _, err := os.Lstat(f); err == nil {
		return
	}
	_ = os.Remove(f + ".old")
	_ = os.Remove(f + ".new")
}

// followRunEntry points the run-at-login entry at the renamed file (a variable: tests never touch the real one).
var followRunEntry = winsys.FollowRename

// setProgramFile tells the window code the new path (a variable: tests leave the package alone).
var setProgramFile = winsys.SetProgramFile

// followName renames the program file when it still has the former name (renameProgramFile), and makes everything
// that names it follow: the path updates and the window use, and the run-at-login entry. It deletes the former name's
// leftovers. It holds off while an update is being installed (the next start does it).
func (a *app) followName() {
	if !a.upd.installing.CompareAndSwap(false, true) {
		return
	}
	defer a.upd.installing.Store(false)
	old := a.exePath()
	now, level, msg := renameProgramFile(runtime.GOOS, runtime.GOARCH, old)
	if msg != "" {
		a.log(level, msg)
	}
	if now != old {
		a.setExe(now)
		setProgramFile(now)
		if note := followRunEntry(old, now); note != "" {
			a.log("INFO", note)
		}
	}
	removeFormerLeftovers(runtime.GOOS, runtime.GOARCH, now)
}
