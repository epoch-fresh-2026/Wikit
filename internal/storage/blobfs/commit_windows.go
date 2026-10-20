package blobfs

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func (s *Store) initialize() error { return nil }

func (s *Store) commit(stage, key string) error {
	// Both keys are server UUID paths. The media root is deployment-owned;
	// verify Root resolution before the Windows write-through rename.
	f, err := s.root.Open(stage)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(filepath.Join(s.directory, filepath.FromSlash(stage)))
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(filepath.Join(s.directory, filepath.FromSlash(key)))
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}
