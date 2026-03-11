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

`procshed.yaml.example` defines two containers connected to the bridge:

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
| networks[].address | yes | IP address in CIDR notation |

## License

This project is licensed under the [MIT License](./LICENSE).
