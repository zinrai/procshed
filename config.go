package main

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Containers map[string]ContainerConfig `yaml:"containers"`
}

type ContainerConfig struct {
	Rootfs   string          `yaml:"rootfs"`
	Command  string          `yaml:"command"`
	Hostname string          `yaml:"hostname"`
	Networks []NetworkConfig `yaml:"networks"`
}

type NetworkConfig struct {
	Bridge  string `yaml:"bridge"`
	Address string `yaml:"address"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validateConfig(cfg *Config) error {
	if len(cfg.Containers) == 0 {
		return fmt.Errorf("no containers defined")
	}

	for name, ct := range cfg.Containers {
		if ct.Rootfs == "" {
			return fmt.Errorf("container %s: rootfs is required", name)
		}

		info, err := os.Stat(ct.Rootfs)
		if err != nil {
			return fmt.Errorf("container %s: rootfs %s: %w", name, ct.Rootfs, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("container %s: rootfs %s is not a directory", name, ct.Rootfs)
		}

		if ct.Command == "" {
			return fmt.Errorf("container %s: command is required", name)
		}

		for i, net := range ct.Networks {
			if net.Bridge == "" {
				return fmt.Errorf("container %s: networks[%d].bridge is required", name, i)
			}
			if net.Address == "" {
				return fmt.Errorf("container %s: networks[%d].address is required", name, i)
			}
		}
	}

	return nil
}
