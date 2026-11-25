package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/CptPakundo/MPRP/internal/core"
)

// logName is the log file's name in the data folder.
var logName = strings.ToLower(core.Name) + ".log"

// formerLogs are the log file names under the program's former name, and what they are called now (LogPath).
var formerLogs = [][2]string{
	{formerLog, logName},
	{formerLog + ".old", logName + ".old"}, // the log moved aside when it grew full
}

// formerLog is the log file's name under the program's former name.
var formerLog = strings.ToLower(core.FormerName) + ".log"

// MoveFormerData carries the data folder of the program's former name (from) over to its own (to), once: only when
// to does not exist yet and from is a folder. The folder is renamed; when that fails (a file in it is held open, or
// the two are on different volumes) it is copied and from is left as it is. A copy goes to a temporary folder first,
// so a half-made one never passes for the real thing. When the copy fails too, nothing is changed and the program
// runs on from this time (the next start tries again).
//
// It gives the folder to use and a sentence for the log ("" when there was nothing to do).
func MoveFormerData(from, to string) (dir, note string) {
	if from == "" || to == "" || exists(to) {
		return to, ""
	}
	if fi, err := os.Stat(from); err != nil || !fi.IsDir() {
		return to, ""
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return from, fmt.Sprintf("The data folder %s could not be moved to %s (%v); it is used where it is, and the next start tries again.", from, to, err)
	}
	err := os.Rename(from, to)
	if err == nil {
		renameLogs(to)
		return to, fmt.Sprintf("Moved the data folder %s to %s (the program's new name).", from, to)
	}
	tmp := to + ".moving"
	_ = os.RemoveAll(tmp) // a copy that an earlier start did not finish
	if cerr := copyDir(from, tmp); cerr != nil {
		_ = os.RemoveAll(tmp)
		return from, fmt.Sprintf("The data folder %s could not be moved to %s (%v) or copied (%v); it is used where it is, and the next start tries again.", from, to, err, cerr)
	}
	if rerr := os.Rename(tmp, to); rerr != nil {
		_ = os.RemoveAll(tmp)
		return from, fmt.Sprintf("The data folder %s could not be moved to %s (%v); it is used where it is, and the next start tries again.", from, to, rerr)
	}
	renameLogs(to)
	return to, fmt.Sprintf("Copied the data folder %s to %s (the program's new name); it could not be moved (%v), so the old folder is left in place.", from, to, err)
}

// exists says whether anything is at path.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// renameLogs gives the log files of the former name the current one. When a file of the current name is there too
// (the copy that a former version updated itself to logs under the new name in the former folder), the former file's
// lines go first in it; either way no file of the former name is left, which nothing would ever rotate or remove
// (a log holds titles).
func renameLogs(dir string) {
	for _, l := range formerLogs {
		old, cur := filepath.Join(dir, l[0]), filepath.Join(dir, l[1])
		switch {
		case !exists(old):
		case !exists(cur):
			_ = os.Rename(old, cur)
		default:
			_ = joinFiles(old, cur)
			_ = os.Remove(old)
		}
	}
}

// joinFiles puts the text of first before that of second, in second (through a temporary file: a failure changes
// nothing). It is used only for logs, so a leftover temporary file after a crash does no harm.
func joinFiles(first, second string) error {
	tmp := second + ".joining"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	for _, p := range []string{first, second} {
		in, err := os.Open(p)
		if err == nil {
			_, err = io.Copy(out, in)
			in.Close()
		}
		if err != nil {
			out.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, second); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// copyDir copies the folder from, with everything in it, to a new folder to. Links are not followed (a data folder
// has none of its own).
func copyDir(from, to string) error {
	return filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("%s is outside %s", path, from)
		}
		target := filepath.Join(to, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o700)
		case d.Type().IsRegular():
			return copyFile(path, target)
		}
		return nil // a link or another special file: left out
	})
}

// copyFile copies one file, keeping its permission bits.
func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
