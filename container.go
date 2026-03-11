package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const stateDir = "/var/local/procshed"

// ContainerState is persisted to disk for each running container.
type ContainerState struct {
	Name      string `json:"name"`
	PID       int    `json:"pid"`
	StartTime uint64 `json:"start_time"`
	Rootfs    string `json:"rootfs"`
	Command   string `json:"command"`
}

func containerDir(name string) string {
	return filepath.Join(stateDir, "containers", name)
}

func stateFilePath(name string) string {
	return filepath.Join(containerDir(name), "state.json")
}

func saveState(name string, state *ContainerState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	return os.WriteFile(stateFilePath(name), data, 0644)
}

func loadState(name string) (*ContainerState, error) {
	data, err := os.ReadFile(stateFilePath(name))
	if err != nil {
		return nil, err
	}
	var state ContainerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshaling state: %w", err)
	}
	return &state, nil
}

// ContainerCreate creates and starts a container.
func ContainerCreate(name string, cfg *ContainerConfig) error {
	dir := containerDir(name)

	if err := cleanStaleState(name); err != nil {
		return err
	}

	ov, err := OverlaySetup(dir, cfg.Rootfs)
	if err != nil {
		return fmt.Errorf("overlay setup: %w", err)
	}

	hostname := cfg.Hostname
	if hostname == "" {
		hostname = name
	}

	cmd, err := startInitProcess(ov.Merged, hostname)
	if err != nil {
		OverlayCleanup(dir)
		return err
	}

	pid := cmd.Process.Pid

	if err := setupContainerNetworks(name, cfg.Networks, pid); err != nil {
		killAndCleanup(cmd, dir)
		return err
	}

	startTime, err := getProcessStartTime(pid)
	if err != nil {
		killAndCleanup(cmd, dir)
		return fmt.Errorf("getting process start time: %w", err)
	}

	state := &ContainerState{
		Name:      name,
		PID:       pid,
		StartTime: startTime,
		Rootfs:    cfg.Rootfs,
		Command:   cfg.Command,
	}
	if err := saveState(name, state); err != nil {
		killAndCleanup(cmd, dir)
		return fmt.Errorf("saving state: %w", err)
	}

	slog.Info("container started", "name", name, "pid", pid)

	go func() {
		cmd.Wait()
	}()

	return nil
}

// cleanStaleState checks for leftover state from a previous run.
// If the container process is still alive (verified by both PID and start time),
// it returns an error. Otherwise, it removes the stale state directory.
func cleanStaleState(name string) error {
	state, err := loadState(name)
	if err != nil {
		// No state file: check if the directory itself is leftover
		dir := containerDir(name)
		if _, statErr := os.Stat(dir); statErr == nil {
			slog.Info("removing leftover container directory", "name", name, "dir", dir)
			os.RemoveAll(dir)
		}
		return nil
	}

	if isContainerProcess(state) {
		return fmt.Errorf("container %s is already running (pid %d)", name, state.PID)
	}

	slog.Info("removing stale container state", "name", name, "pid", state.PID)
	ContainerDelete(name)
	return nil
}

// isContainerProcess checks whether the process recorded in the state is
// still the same process that procshed started. It verifies both that the
// PID exists and that its kernel start time matches the recorded value.
// This prevents false positives from PID reuse after a reboot.
func isContainerProcess(state *ContainerState) bool {
	if err := syscall.Kill(state.PID, 0); err != nil {
		return false
	}

	currentStartTime, err := getProcessStartTime(state.PID)
	if err != nil {
		return false
	}

	return currentStartTime == state.StartTime
}

// getProcessStartTime reads the process start time (field 22) from
// /proc/<pid>/stat. This value is measured in clock ticks since boot
// and is unique per PID lifecycle, making it reliable for detecting
// PID reuse across reboots.
func getProcessStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}

	// /proc/<pid>/stat format: pid (comm) state fields...
	// Field 22 (1-indexed) is starttime. Find the closing ')' of comm
	// first, since comm can contain spaces and parentheses.
	content := string(data)
	closeParen := strings.LastIndex(content, ")")
	if closeParen < 0 {
		return 0, fmt.Errorf("unexpected format in /proc/%d/stat", pid)
	}

	// Fields after ") " start at field 3
	fields := strings.Fields(content[closeParen+2:])
	if len(fields) < 20 {
		return 0, fmt.Errorf("not enough fields in /proc/%d/stat", pid)
	}

	// starttime is field 22 overall, which is index 19 in the fields after ")"
	// (fields after ")" start at field 3, so field 22 = index 22-3 = 19)
	startTime, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing starttime from /proc/%d/stat: %w", pid, err)
	}

	return startTime, nil
}

// startInitProcess re-execs the procshed binary with the "init" command
// inside new namespaces.
func startInitProcess(rootfs, hostname string) (*exec.Cmd, error) {
	initCfg := InitConfig{
		Rootfs:   rootfs,
		Hostname: hostname,
	}
	initData, err := json.Marshal(initCfg)
	if err != nil {
		return nil, fmt.Errorf("marshaling init config: %w", err)
	}

	cmd := exec.Command("/proc/self/exe", "init")
	cmd.Env = append(os.Environ(), "PROCSHED_INIT="+string(initData))
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWNET,
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting container process: %w", err)
	}

	return cmd, nil
}

// setupContainerNetworks creates veth pairs and connects the container to bridges.
func setupContainerNetworks(name string, networks []NetworkConfig, pid int) error {
	for i, net := range networks {
		vethHost := VethName(name, i)
		vethContainer := fmt.Sprintf("eth%d", i)
		if err := NetworkSetup(vethHost, vethContainer, net.Bridge, net.Address, pid); err != nil {
			return fmt.Errorf("network setup [%d]: %w", i, err)
		}
	}
	return nil
}

// killAndCleanup terminates the init process and removes the overlay.
func killAndCleanup(cmd *exec.Cmd, dir string) {
	cmd.Process.Kill()
	cmd.Wait()
	OverlayCleanup(dir)
}

// ContainerDelete stops and removes a container.
func ContainerDelete(name string) error {
	dir := containerDir(name)

	// Kill process if running
	if state, err := loadState(name); err == nil {
		if isContainerProcess(state) {
			slog.Info("killing container process", "name", name, "pid", state.PID)
			syscall.Kill(state.PID, syscall.SIGTERM)
			syscall.Kill(state.PID, syscall.SIGKILL)
		}
	}

	// Clean up network (try indices 0-99)
	for i := 0; i < 100; i++ {
		vethHost := VethName(name, i)
		if !NetworkCleanup(vethHost) {
			break
		}
	}

	// Clean up overlay
	OverlayCleanup(dir)

	// Remove container directory
	os.RemoveAll(dir)

	return nil
}

// ContainerList returns all containers with state files.
func ContainerList() ([]ContainerState, error) {
	containersDir := filepath.Join(stateDir, "containers")
	entries, err := os.ReadDir(containersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []ContainerState
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		state, err := loadState(entry.Name())
		if err != nil {
			continue
		}
		if isContainerProcess(state) {
			result = append(result, *state)
		}
	}

	return result, nil
}

// ContainerExec runs a command inside an existing container's namespaces.
func ContainerExec(name string, command []string) error {
	state, err := loadState(name)
	if err != nil {
		return fmt.Errorf("container %s not found: %w", name, err)
	}

	if !isContainerProcess(state) {
		return fmt.Errorf("container %s is not running", name)
	}

	// Enter the container's namespaces via nsenter
	args := []string{
		"-t", strconv.Itoa(state.PID),
		"-m", "-p", "-u", "-i", "-n",
		"--",
	}
	args = append(args, command...)

	cmd := exec.Command("nsenter", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
