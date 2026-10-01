package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Recover removes only recognized remnants of interrupted snapshot writes or
// deletions. The store lock prevents removal of an operation still in progress.
// Unexpected contents are retained for manual inspection.
func (s checkpointStore) Recover() error {
	root, err := s.open(false)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	guard, err := lockMoveStore(moveStore{dir: s.path()})
	if err != nil {
		return err
	}
	defer guard.Close()
	return s.cleanupInterrupted(root)
}

func (s checkpointStore) cleanupInterrupted(root *os.Root) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, err := dir.ReadDir(4097)
	dir.Close()
	if err != nil && err != io.EOF {
		return err
	}
	if len(entries) > 4096 {
		return fmt.Errorf("snapshot store has too many entries")
	}
	for _, entry := range entries {
		name := entry.Name()
		id := strings.TrimPrefix(name, ".pending-")
		pending := id != name
		if !pending {
			id = strings.TrimPrefix(name, ".deleting-")
		}
		if id == name || !validCheckpointID(id) {
			continue
		}
		info, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if !info.IsDir() || rejectMoveLink(filepath.Join(s.path(), name), info) != nil {
			continue
		}
		stage, err := root.Open(name)
		if err != nil {
			return err
		}
		files, readErr := stage.ReadDir(4)
		stage.Close()
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if len(files) > 3 {
			continue
		}
		recognized := true
		for _, file := range files {
			allowed := file.Name() == "vm.zip" || file.Name() == "snapshot.json"
			if pending && strings.HasPrefix(file.Name(), ".try-omarchy-backup-") {
				// Older launchers named the staging file with decimal digits;
				// current ones use a random hexadecimal token.
				suffix := strings.TrimPrefix(file.Name(), ".try-omarchy-backup-")
				allowed = suffix != "" && strings.Trim(suffix, "0123456789abcdef") == ""
			}
			info, err := file.Info()
			if err != nil {
				return err
			}
			if !allowed || !info.Mode().IsRegular() || rejectMoveLink(filepath.Join(s.path(), name, file.Name()), info) != nil {
				recognized = false
				break
			}
		}
		if !recognized {
			continue
		}
		if err := root.RemoveAll(name); err != nil {
			return err
		}
	}
	return nil
}
