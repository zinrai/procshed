package main

import (
	"net"
	"testing"

	"github.com/vishvananda/netlink"
)

func TestVethName(t *testing.T) {
	tests := []struct {
		name      string
		container string
		index     int
		want      string
	}{
		{
			name:      "example from procshed.yaml",
			container: "web",
			index:     0,
			want:      "vp-4b5e57f6-0",
		},
		{
			name:      "second container",
			container: "app",
			index:     0,
			want:      "vp-a172cedc-0",
		},
		{
			name:      "second network index",
			container: "web",
			index:     1,
			want:      "vp-4b5e57f6-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VethName(tt.container, tt.index)
			if got != tt.want {
				t.Errorf("VethName(%q, %d) = %q, want %q", tt.container, tt.index, got, tt.want)
			}
		})
	}
}

func TestVethNameLength(t *testing.T) {
	// IFNAMSIZ is 16 bytes including null terminator, so max 15 characters.
	const maxLen = 15

	containers := []string{
		"a",
		"web",
		"my-long-container-name-that-exceeds-normal-length",
		"",
	}
	indices := []int{0, 1, 9, 10, 99}

	for _, name := range containers {
		for _, idx := range indices {
			got := VethName(name, idx)
			if len(got) > maxLen {
				t.Errorf("VethName(%q, %d) = %q (len=%d), exceeds IFNAMSIZ limit of %d",
					name, idx, got, len(got), maxLen)
			}
		}
	}
}

func TestVethNameDeterministic(t *testing.T) {
	first := VethName("web", 0)
	second := VethName("web", 0)
	if first != second {
		t.Errorf("VethName is not deterministic: %q != %q", first, second)
	}
}

func TestDefaultGateway(t *testing.T) {
	tests := []struct {
		name string
		cidr string
		want string
	}{
		{
			name: "/24 subnet",
			cidr: "10.0.1.1/24",
			want: "10.0.1.254",
		},
		{
			name: "/16 subnet",
			cidr: "172.16.0.1/16",
			want: "172.16.255.254",
		},
		{
			name: "/30 subnet",
			cidr: "192.168.1.100/30",
			want: "192.168.1.102",
		},
		{
			name: "/8 subnet",
			cidr: "10.1.2.3/8",
			want: "10.255.255.254",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := netlink.ParseAddr(tt.cidr)
			if err != nil {
				t.Fatalf("ParseAddr(%q): %v", tt.cidr, err)
			}

			got := defaultGateway(addr)
			if got == nil {
				t.Fatalf("defaultGateway returned nil for %s", tt.cidr)
			}

			want := net.ParseIP(tt.want)
			if !got.Equal(want) {
				t.Errorf("defaultGateway(%s) = %s, want %s", tt.cidr, got, want)
			}
		})
	}
}

func TestDefaultGatewayNilIPNet(t *testing.T) {
	addr := &netlink.Addr{IPNet: nil}
	got := defaultGateway(addr)
	if got != nil {
		t.Errorf("defaultGateway with nil IPNet = %s, want nil", got)
	}
}

func TestValidateConfigAddressOptional(t *testing.T) {
	// A network entry without an address must be accepted: the link is
	// brought up but no IP is assigned.
	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{
			name:    "valid CIDR",
			address: "10.0.1.1/24",
			wantErr: false,
		},
		{
			name:    "empty address (no-IP mode)",
			address: "",
			wantErr: false,
		},
		{
			name:    "invalid CIDR",
			address: "10.0.1.1/33",
			wantErr: true,
		},
		{
			name:    "not a CIDR",
			address: "not-an-address",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Containers: map[string]ContainerConfig{
					"test": {
						Rootfs:  "/", // existing directory; validateConfig stats it
						Command: "/bin/sleep infinity",
						Networks: []NetworkConfig{
							{Bridge: "vm0", Address: tt.address},
						},
					},
				},
			}
			err := validateConfig(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
