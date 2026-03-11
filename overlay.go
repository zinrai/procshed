package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Overlay holds the paths for an overlayfs mount.
type Overlay struct {
	Lower  string // Original rootfs (read-only)
	Upper  string // Container-specific writable layer
	Work   string // overlayfs work directory
	Merged string // Mount point (union of lower + upper)
}

// OverlaySetup creates directories and mounts overlayfs.
func OverlaySetup(containerDir string, rootfs string) (*Overlay, error) {
	ov := &Overlay{
		Lower:  rootfs,
		Upper:  filepath.Join(containerDir, "upper"),
		Work:   filepath.Join(containerDir, "work"),
		Merged: filepath.Join(containerDir, "merged"),
	}

	dirs := []string{ov.Upper, ov.Work, ov.Merged}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("creating %s: %w", d, err)
		}
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", ov.Lower, ov.Upper, ov.Work)
	if err := syscall.Mount("overlay", ov.Merged, "overlay", 0, opts); err != nil {
		return nil, fmt.Errorf("mounting overlayfs: %w", err)
	}

	return ov, nil
}

// OverlayCleanup unmounts overlayfs and removes directories.
func OverlayCleanup(containerDir string) {
	merged := filepath.Join(containerDir, "merged")

	// Attempt to unmount; ignore errors if not mounted
	syscall.Unmount(merged, syscall.MNT_DETACH)
}
