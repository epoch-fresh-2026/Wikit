//go:build !windows

package blobfs

import "errors"

func (s *Store) initialize() error { return s.syncDir(".") }

func (s *Store) syncDir(name string) error {
	f, err := s.root.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
func (s *Store) commit(stage, key string) error {
	if err := s.root.Link(stage, key); err != nil {
		return err
	}
	if err := s.syncDir("objects"); err != nil {
		return err
	}
	if err := s.root.Remove(stage); err != nil {
		return err
	}
	return s.syncDir("staging")
}
