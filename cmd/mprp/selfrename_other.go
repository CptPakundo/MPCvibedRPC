//go:build !windows

package main

import "os"

// renameNoReplace renames from to to, failing when to exists (os.Rename would replace it). A hard link fails when to
// exists; a file system without hard links gets a plain rename after a look.
func renameNoReplace(from, to string) error {
	if err := os.Link(from, to); err == nil {
		if err := os.Remove(from); err != nil {
			_ = os.Remove(to)
			return err
		}
		return nil
	}
	if _, err := os.Lstat(to); err == nil {
		return os.ErrExist
	}
	return os.Rename(from, to)
}
