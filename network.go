package main

import (
	"crypto/sha256"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// VethName generates a host-side veth interface name from the container name
// and network index. The format is "vp-<hash>-<index>" where hash is the
// first 8 characters of the SHA-256 hex digest of the container name.
// This keeps the name within the 15-character IFNAMSIZ limit.
func VethName(containerName string, index int) string {
	h := sha256.Sum256([]byte(containerName))
	return fmt.Sprintf("vp-%x-%d", h[:4], index)
}

// NetworkSetup creates a veth pair, connects one end to the bridge,
// and moves the other end into the container's network namespace
// with the specified IP address.
func NetworkSetup(vethHost, vethContainer, bridgeName, address string, pid int) error {
	// Parse the CIDR address
	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parsing address %s: %w", address, err)
	}

	// Find the bridge
	br, err := netlink.LinkByName(bridgeName)
	if err != nil {
		return fmt.Errorf("finding bridge %s: %w", bridgeName, err)
	}

	// Create veth pair
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{
			Name: vethHost,
		},
		PeerName: vethContainer,
	}
	if err := netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("creating veth pair: %w", err)
	}

	// Connect host side to bridge
	hostLink, err := netlink.LinkByName(vethHost)
	if err != nil {
		return fmt.Errorf("finding %s: %w", vethHost, err)
	}
	if err := netlink.LinkSetMaster(hostLink, br); err != nil {
		netlink.LinkDel(hostLink)
		return fmt.Errorf("connecting %s to bridge %s: %w", vethHost, bridgeName, err)
	}
	if err := netlink.LinkSetUp(hostLink); err != nil {
		netlink.LinkDel(hostLink)
		return fmt.Errorf("setting %s up: %w", vethHost, err)
	}

	// Move container side into the container's network namespace
	containerLink, err := netlink.LinkByName(vethContainer)
	if err != nil {
		netlink.LinkDel(hostLink)
		return fmt.Errorf("finding %s: %w", vethContainer, err)
	}
	if err := netlink.LinkSetNsPid(containerLink, pid); err != nil {
		netlink.LinkDel(hostLink)
		return fmt.Errorf("moving %s to pid %d ns: %w", vethContainer, pid, err)
	}

	// Enter the container's network namespace to configure the interface
	nsHandle, err := netns.GetFromPid(pid)
	if err != nil {
		netlink.LinkDel(hostLink)
		return fmt.Errorf("getting netns for pid %d: %w", pid, err)
	}
	defer nsHandle.Close()

	if err := configureContainerNetwork(nsHandle, vethContainer, addr); err != nil {
		netlink.LinkDel(hostLink)
		return err
	}

	return nil
}

// configureContainerNetwork enters the container netns and configures
// the network interface with an IP address and brings it up.
func configureContainerNetwork(nsHandle netns.NsHandle, ifName string, addr *netlink.Addr) error {
	// Save current namespace
	origNs, err := netns.Get()
	if err != nil {
		return fmt.Errorf("getting current netns: %w", err)
	}
	defer origNs.Close()
	defer netns.Set(origNs)

	// Switch to container namespace
	if err := netns.Set(nsHandle); err != nil {
		return fmt.Errorf("setting netns: %w", err)
	}

	// Bring up loopback
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("finding lo: %w", err)
	}
	if err := netlink.LinkSetUp(lo); err != nil {
		return fmt.Errorf("setting lo up: %w", err)
	}

	// Configure the container interface
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("finding %s in container: %w", ifName, err)
	}

	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("adding address to %s: %w", ifName, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("setting %s up: %w", ifName, err)
	}

	// Add default route via the gateway (first IP in subnet)
	gw := defaultGateway(addr)
	if gw != nil {
		route := &netlink.Route{
			Gw: gw,
		}
		if err := netlink.RouteAdd(route); err != nil {
			return fmt.Errorf("adding default route: %w", err)
		}
	}

	return nil
}

// defaultGateway derives the gateway address from the container's IP.
// Convention: the gateway is the last usable address in the subnet (.254 for /24).
// This matches netshed's bridge address convention.
func defaultGateway(addr *netlink.Addr) net.IP {
	network := addr.IPNet
	if network == nil {
		return nil
	}

	ones, bits := network.Mask.Size()
	if ones == 0 || bits == 0 {
		return nil
	}

	// Calculate the broadcast address, then subtract 1
	ip := make(net.IP, len(network.IP))
	copy(ip, network.IP)

	// For IPv4
	if ip4 := ip.To4(); ip4 != nil {
		// Set host bits to get broadcast
		hostBits := uint(bits - ones)
		broadcast := make(net.IP, 4)
		copy(broadcast, ip4)
		for i := uint(0); i < hostBits; i++ {
			byteIdx := 3 - i/8
			bitIdx := i % 8
			broadcast[byteIdx] |= 1 << bitIdx
		}
		// Gateway = broadcast - 1
		broadcast[3]--
		return broadcast
	}

	return nil
}

// NetworkCleanup removes the host-side veth.
// Deleting one end of a veth pair automatically deletes the other.
// Returns true if the veth existed and was deleted.
func NetworkCleanup(vethHost string) bool {
	link, err := netlink.LinkByName(vethHost)
	if err != nil {
		return false
	}
	netlink.LinkDel(link)
	return true
}
