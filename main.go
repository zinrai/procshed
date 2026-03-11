package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
)

const usage = `Usage: procshed <command> [options]

Commands:
  create    Create and start containers from config
  delete    Stop and remove containers from config
  list      List running containers
  exec      Execute a command in a running container

Options:
  -config string  Path to config file (default "containers.yaml")
`

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "create":
		cmdCreate(os.Args[2:])
	case "delete":
		cmdDelete(os.Args[2:])
	case "list":
		cmdList(os.Args[2:])
	case "exec":
		cmdExec(os.Args[2:])
	case "init":
		// Re-exec entry point for child process inside namespace.
		// This is called internally and not exposed to the user.
		cmdInit()
	default:
		slog.Error("unknown command", "command", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
}

func cmdCreate(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	configPath := fs.String("config", "containers.yaml", "path to config file")
	fs.Parse(args)

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	for name, ct := range cfg.Containers {
		slog.Info("creating container", "name", name)
		if err := ContainerCreate(name, &ct); err != nil {
			slog.Error("failed to create container", "name", name, "error", err)
			os.Exit(1)
		}
	}
}

func cmdDelete(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	configPath := fs.String("config", "containers.yaml", "path to config file")
	fs.Parse(args)

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	for name := range cfg.Containers {
		slog.Info("deleting container", "name", name)
		if err := ContainerDelete(name); err != nil {
			slog.Error("failed to delete container", "name", name, "error", err)
		}
	}
}

func cmdList(args []string) {
	containers, err := ContainerList()
	if err != nil {
		slog.Error("failed to list containers", "error", err)
		os.Exit(1)
	}

	if len(containers) == 0 {
		fmt.Println("No running containers")
		return
	}

	fmt.Printf("%-20s %-10s %s\n", "NAME", "PID", "DIR")
	for _, c := range containers {
		fmt.Printf("%-20s %-10d %s\n", c.Name, c.PID, containerDir(c.Name))
	}
}

func cmdExec(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: procshed exec <container-name> <command> [args...]")
		os.Exit(1)
	}

	name := args[0]
	command := args[1:]

	if err := ContainerExec(name, command); err != nil {
		slog.Error("failed to exec", "name", name, "error", err)
		os.Exit(1)
	}
}
