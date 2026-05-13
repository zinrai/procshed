# procshed

Lightweight container tool using Linux namespaces and overlayfs. Sits between chroot and Docker — provides namespace isolation without the full container platform.

## Overview

procshed creates isolated containers from a rootfs directory (e.g., debootstrap output) using PID, Mount, UTS, IPC, and Net namespaces with overlayfs for copy-on-write storage. Network infrastructure (bridges, masquerade) is delegated to [netshed](https://github.com/zinrai/netshed).

## Requirements

- Linux kernel with overlayfs, namespaces support
- Root privileges
- [netshed](https://github.com/zinrai/netshed)

## Quick Start

### 1. Prepare rootfs

```bash
$ sudo mkdir -p /var/local/procshed/rootfs
$ sudo debootstrap --variant=minbase bookworm /var/local/procshed/rootfs/bookworm
```

### 2. Create network infrastructure

`netshed.yaml.example` defines a bridge with masquerade for internet access:

```bash
$ sudo netshed create -config netshed.yaml.example
```

### 3. Create containers

`procshed.yaml.example` defines three containers connected to the bridge. Two are assigned static IP addresses; one is attached without an IP address.

```bash
$ sudo procshed create -config procshed.yaml.example
```

### 4. Use containers

List running containers

```bash
$ sudo procshed list
```

Execute command in a container

```bash
$ sudo procshed exec web /bin/bash
```

### 5. Tear down

```bash
$ sudo procshed delete -config procshed.yaml.example
$ sudo netshed delete -config netshed.yaml.example
```

## Configuration

| Field | Required | Description |
|-------|----------|-------------|
| rootfs | yes | Path to rootfs directory (overlayfs lowerdir) |
| command | yes | Command to run inside the container |
| hostname | no | Container hostname (defaults to container name) |
| networks | no | List of network connections |
| networks[].bridge | yes | Existing bridge to connect to |
| networks[].address | no | IP address in CIDR notation. If omitted, the interface is brought up without an address or default route |

### Network without IP assignment

To attach a container to a bridge without assigning an IP address, omit the `address` field. The veth pair is created and brought up, but no address or default route is configured inside the container. This is useful when the container is expected to assign its own address from inside (for example, from the `command` itself).

```yaml
containers:
  raw:
    rootfs: /var/local/procshed/rootfs/bookworm
    command: /bin/sleep infinity
    networks:
      - bridge: vm0
```

## License

This project is licensed under the [MIT License](./LICENSE).
