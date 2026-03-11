package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Device files to bind mount from host into the container.
var bindDevices = []string{
	"/dev/null",
	"/dev/zero",
	"/dev/random",
	"/dev/urandom",
	"/dev/tty",
}

// SetupMounts prepares /proc, /dev, /sys inside the new rootfs.
// This is called from the child process inside the new mount namespace,
// before pivot_root.
func SetupMounts(rootfs string) error {
	if err := setupDev(rootfs); err != nil {
		return fmt.Errorf("setting up /dev: %w", err)
	}

	if err := setupProc(rootfs); err != nil {
		return fmt.Errorf("setting up /proc: %w", err)
	}

	if err := setupSys(rootfs); err != nil {
		return fmt.Errorf("setting up /sys: %w", err)
	}

	return nil
}

func setupDev(rootfs string) error {
	devDir := filepath.Join(rootfs, "dev")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		return err
	}

	// Bind mount individual device files
	for _, dev := range bindDevices {
		target := filepath.Join(rootfs, dev)

		// Create the target file if it doesn't exist
		if err := touchFile(target); err != nil {
			return fmt.Errorf("creating %s: %w", target, err)
		}

		if err := syscall.Mount(dev, target, "", syscall.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind mount %s: %w", dev, err)
		}
	}

	// Mount devpts for pseudo-terminals
	ptsDir := filepath.Join(rootfs, "dev", "pts")
	if err := os.MkdirAll(ptsDir, 0755); err != nil {
		return err
	}
	if err := syscall.Mount("devpts", ptsDir, "devpts", 0, "newinstance,ptmxmode=0666"); err != nil {
		return fmt.Errorf("mounting devpts: %w", err)
	}

	// Symlink /dev/ptmx to /dev/pts/ptmx
	ptmx := filepath.Join(rootfs, "dev", "ptmx")
	os.Remove(ptmx) // Remove if exists
	if err := os.Symlink("pts/ptmx", ptmx); err != nil {
		return fmt.Errorf("symlink ptmx: %w", err)
	}

	return nil
}

func setupProc(rootfs string) error {
	procDir := filepath.Join(rootfs, "proc")
	if err := os.MkdirAll(procDir, 0755); err != nil {
		return err
	}

	// Mount a new /proc for the PID namespace
	if err := syscall.Mount("proc", procDir, "proc", 0, ""); err != nil {
		return fmt.Errorf("mounting /proc: %w", err)
	}

	return nil
}

func setupSys(rootfs string) error {
	sysDir := filepath.Join(rootfs, "sys")
	if err := os.MkdirAll(sysDir, 0755); err != nil {
		return err
	}

	// Bind mount /sys as read-only
	if err := syscall.Mount("/sys", sysDir, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind mount /sys: %w", err)
	}

	// Remount as read-only
	if err := syscall.Mount("", sysDir, "", syscall.MS_BIND|syscall.MS_REMOUNT|syscall.MS_RDONLY|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("remount /sys read-only: %w", err)
	}

	return nil
}

func touchFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}
