package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// InitConfig is passed from the parent process via environment variable.
type InitConfig struct {
	Rootfs   string `json:"rootfs"`
	Hostname string `json:"hostname"`
}

// cmdInit is the entry point for the child process after clone.
// It runs inside the new namespaces and performs:
//   - make-rprivate
//   - mount /proc, /dev, /sys
//   - pivot_root
//   - set hostname
//   - exec the user command
func cmdInit() {
	data := os.Getenv("PROCSHED_INIT")
	if data == "" {
		slog.Error("PROCSHED_INIT not set")
		os.Exit(1)
	}

	var cfg InitConfig
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		slog.Error("parsing init config", "error", err)
		os.Exit(1)
	}

	if err := nsInit(&cfg); err != nil {
		slog.Error("init failed", "error", err)
		os.Exit(1)
	}
}

func nsInit(cfg *InitConfig) error {
	// Prevent mount propagation to host
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("make-rprivate: %w", err)
	}

	// Setup mounts inside the new rootfs
	if err := SetupMounts(cfg.Rootfs); err != nil {
		return fmt.Errorf("setup mounts: %w", err)
	}

	// pivot_root to the new rootfs
	if err := pivotRoot(cfg.Rootfs); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}

	// Set hostname
	if err := syscall.Sethostname([]byte(cfg.Hostname)); err != nil {
		return fmt.Errorf("sethostname: %w", err)
	}

	// Wait for network setup by sleeping briefly.
	// The parent process sets up veth after this process starts.
	// The user command will be executed by the parent via nsenter,
	// so this init process just keeps the namespaces alive.
	select {}
}

func pivotRoot(newRoot string) error {
	// pivot_root requires the new root and the old root to be
	// on different mount points. Bind mount the new root onto itself.
	if err := syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind mount new root: %w", err)
	}

	// Create directory for old root
	oldRoot := filepath.Join(newRoot, ".pivot_old")
	if err := os.MkdirAll(oldRoot, 0700); err != nil {
		return fmt.Errorf("mkdir old root: %w", err)
	}

	// pivot_root
	if err := syscall.PivotRoot(newRoot, oldRoot); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}

	// Change to new root
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("chdir: %w", err)
	}

	// Unmount old root
	if err := syscall.Unmount("/.pivot_old", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("unmount old root: %w", err)
	}

	// Remove old root mount point
	os.Remove("/.pivot_old")

	return nil
}

// RunInContainer executes the user command inside the container.
// This is used when the container runs a long-lived process directly.
func RunInContainer(command string) error {
	shell := "/bin/sh"
	args := []string{shell, "-c", command}

	binary, err := exec.LookPath(shell)
	if err != nil {
		return fmt.Errorf("looking up %s: %w", shell, err)
	}

	return syscall.Exec(binary, args, os.Environ())
}
