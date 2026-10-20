// Package blobfs implements the local media filesystem boundary.
package blobfs

import (
	"context"
	"errors"
	"os"
)

type Probe struct{ Root string }

// Check writes a unique disposable file, syncs it and removes it. Local filesystem
// syscalls cannot be interrupted; context is checked between these operations.
func (p Probe) Check(ctx context.Context) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.CreateTemp(p.Root, ".wikit-health-*")
	if err != nil {
		return errors.New("media directory unavailable")
	}
	defer func() { result = errors.Join(result, os.Remove(f.Name())) }()
	_, writeErr := f.Write([]byte("health\n"))
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(writeErr, syncErr, closeErr, ctx.Err())
}
