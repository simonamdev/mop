# mop

**mop** is a lightweight TCP proxy designed to wake up machines on-demand. It sits between your client and your destination, intercepting connection attempts to transparently power on the target, either via Wake-on-LAN or the Proxmox API—before proxying the traffic.

## Why mop?

`mop` was created to solve a personal homelab problem: wanting to turn on a machine when I try to connect to it, without having to manually power it on or run a Wake-on-LAN script.

## Usage

### Run with Docker

Pre-built images are available on GitHub Container Registry for both amd64 and arm64.

You can run `mop` using `docker run`:

```bash
docker run -d \
  --name mop \
  --restart unless-stopped \
  -p 2222:2222 \
  --env-file .env \
  ghcr.io/simonamdev/mop:latest
```

Make sure your `.env` file is properly configured as described in the [Configuration](#configuration) section below.

### Configuration

`mop` is configured entirely via environment variables. You can define these in a `.env` file in the working directory.

#### Common Settings
| Variable | Description | Default |
|----------|-------------|---------|
| `PROXY_PORT` | The port `mop` listens on locally. | `2222` |
| `TARGET_HOST` | The IP address or hostname of the target machine. | *(Required)* |
| `TARGET_PORT` | The port on the target machine (e.g., 22 for SSH). | `22` |
| `CONNECTION_RETRIES`| Number of times to retry connecting after wakeup. | `15` |
| `RETRY_DELAY_SECONDS`| Seconds to wait between connection retries. | `5` |

#### Wake-on-LAN (WOL)

Set `WAKEUP_METHOD=wol`.

| Variable | Description |
|----------|-------------|
| `TARGET_MAC` | The MAC address of the target machine. |
| `TARGET_BROADCAST_IP` | Broadcast IP for the network (usually ends in .255). |

#### Proxmox VE

Set `WAKEUP_METHOD=proxmox`.

| Variable | Description |
|----------|-------------|
| `PROXMOX_API_URL` | Full URL to the Proxmox API (e.g., `https://host:8006/api2/json`). |
| `PROXMOX_NODE` | The name of the Proxmox node containing the VM/CT. |
| `PROXMOX_VMID` | The ID of the VM or Container (e.g., `100`). |
| `PROXMOX_TOKEN` | API Token in format `user@pam!tokenid=uuid-secret`. |
| `PROXMOX_TYPE` | `qemu` (for VMs) or `lxc` (for LXC Containers). |
| `PROXMOX_INSECURE`| Set to `true` to skip SSL verification. |

### Development

To run `mop` locally for development:

1. **Clone the repository:**

```bash
git clone https://github.com/simonamdev/mop.git
cd mop
```

2. **Configure your environment:**

Create a `.env` file. You can start by copying an example:

```bash
cp .env.wol-example .env
# Open .env and adjust the variables to match your setup
```

3. **Run the server:**

```bash
go run .
```

4. **Connect:**

Point your SSH client (or other TCP client) to the proxy:
```bash
ssh -p 2222 user@localhost
```

## License

Licensed under the Apache License, Version 2.0.
