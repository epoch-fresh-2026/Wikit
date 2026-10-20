package blobfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProbe(t *testing.T) {
	root := t.TempDir()
	if err := (Probe{root}).Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 0 {
		t.Fatal("probe left files behind", err)
	}
	if err := (Probe{filepath.Join(root, "missing")}).Check(context.Background()); err == nil {
		t.Fatal("accepted missing directory")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (Probe{root}).Check(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
}
