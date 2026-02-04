# k4s

A Terminal User Interface (TUI) for K3s cluster management, inspired by [k9s](https://k9scli.io/).

![k4s demo](https://img.shields.io/badge/version-0.2.0-blue)
![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-green)

## Features

- **Multiple Kubeconfig Support** - Manage multiple K3s/Kubernetes clusters from a single config
- **Real-time Pod Monitoring** - Live-updating pod list with status colors
- **Deployments Management** - View, scale, restart, and delete deployments
- **Services View** - List services with type, IPs, and ports
- **Events Browser** - Cluster-wide events with filtering (warnings, kind)
- **Resource Metrics** - CPU/Memory usage for pods (requires metrics-server)
- **Streaming Logs** - View and follow pod logs in real-time (like `kubectl logs -f`)
- **Pod Operations** - Delete and restart pods with confirmation dialogs
- **SSH Integration** - Connect to nodes via SSH and run crictl commands
- **Container Runtime Inspection** - View containers directly via crictl on nodes
- **Search in Logs** - Find specific entries in log output with highlighting
- **Keyboard-driven** - Efficient navigation with vim-style keybindings

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/LywwKkA-aD/k4s.git
cd k4s

# Build and install
make install

# Or just build
make build
./build/k4s
```

### From Release

Download the latest release for your platform from the [Releases](https://github.com/LywwKkA-aD/k4s/releases) page.

```bash
# Linux (amd64)
curl -LO https://github.com/LywwKkA-aD/k4s/releases/download/v0.2.0/k4s-v0.2.0-linux-amd64.tar.gz
tar -xzf k4s-v0.2.0-linux-amd64.tar.gz
sudo mv k4s-linux-amd64 /usr/local/bin/k4s

# macOS (Apple Silicon)
curl -LO https://github.com/LywwKkA-aD/k4s/releases/download/v0.2.0/k4s-v0.2.0-darwin-arm64.tar.gz
tar -xzf k4s-v0.2.0-darwin-arm64.tar.gz
sudo mv k4s-darwin-arm64 /usr/local/bin/k4s
```

## Configuration

k4s uses a configuration file at `~/.k4s/config.yaml`. On first run, a default configuration is created.

### Example Configuration

```yaml
kubeconfigs:
  - name: "local-k3s"
    path: "~/.kube/config"
    default: true
  - name: "production"
    path: "~/.kube/prod-config"

ssh_hosts:
  - name: "k3s-node-1"
    host: "192.168.1.100"
    user: "admin"
    key_path: "~/.ssh/id_rsa"
    port: 22
  - name: "k3s-node-2"
    host: "192.168.1.101"
    user: "admin"
    key_path: "~/.ssh/id_rsa"
```

### Configuration Options

| Field | Description |
|-------|-------------|
| `kubeconfigs` | List of Kubernetes configuration files |
| `kubeconfigs[].name` | Display name for the cluster |
| `kubeconfigs[].path` | Path to kubeconfig file (supports `~`) |
| `kubeconfigs[].default` | Set to `true` for auto-selection |
| `ssh_hosts` | List of SSH hosts for crictl access |
| `ssh_hosts[].name` | Display name for the node |
| `ssh_hosts[].host` | Hostname or IP address |
| `ssh_hosts[].user` | SSH username |
| `ssh_hosts[].key_path` | Path to SSH private key |
| `ssh_hosts[].port` | SSH port (default: 22) |

## Usage

```bash
# Start k4s
k4s

# Show version
k4s --version
k4s -v
```

## Keyboard Shortcuts

Press `?` at any time to see the help screen.

### Global

| Key | Action |
|-----|--------|
| `?` | Show/hide help |
| `q` | Quit |
| `Ctrl+C` | Force quit |
| `Esc` | Go back / Cancel |
| `r` | Refresh current view |

### Navigation

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `Enter` | Select / Open |
| `/` | Filter list / Search |
| `1` | Go to Namespaces |
| `2` | Go to Pods |
| `3` | Go to Deployments |
| `4` | Go to Services |
| `5` | Go to Events |
| `9` | Go to SSH Hosts |

### Pod Actions

| Key | Action |
|-----|--------|
| `l` | View logs |
| `d` | Delete pod |
| `R` | Restart pod (Shift+R) |
| `m` | Toggle metrics |

### Deployment Actions

| Key | Action |
|-----|--------|
| `s` | Scale deployment |
| `d` | Delete deployment |
| `R` | Restart deployment |

### Events Actions

| Key | Action |
|-----|--------|
| `f` | Toggle follow mode |
| `w` | Toggle warnings only |
| `k` | Cycle kind filter |

### Log Viewer

| Key | Action |
|-----|--------|
| `f` | Toggle follow mode |
| `t` | Toggle timestamps |
| `/` | Search in logs |
| `n` | Next search match |
| `N` | Previous search match |
| `g` | Go to top |
| `G` | Go to bottom |
| `c` | Change container |

### Scrolling

| Key | Action |
|-----|--------|
| `↑` / `↓` | Scroll line by line |
| `PgUp` / `PgDn` | Scroll page |
| `Home` / `g` | Go to top |
| `End` / `G` | Go to bottom |

## Views

### Namespaces View (`1`)
Browse and select Kubernetes namespaces. Shows namespace status and age.

### Pods View (`2`)
List all pods in the selected namespace with:
- Pod name
- Ready containers (X/Y)
- Status (color-coded)
- Restart count
- Age
- CPU/Memory (toggle with `m`, requires metrics-server)

Auto-refreshes every 5 seconds.

### Deployments View (`3`)
List all deployments with:
- Deployment name
- Ready/Desired replicas
- Up-to-date count
- Available count
- Age

Actions: scale (`s`), restart (`R`), delete (`d`)

### Services View (`4`)
List all services with:
- Service name
- Type (ClusterIP, NodePort, LoadBalancer)
- Cluster IP
- External IP
- Ports
- Age

### Events View (`5`)
Cluster-wide events in log-style format:
- Auto-follow mode (`f`)
- Filter warnings only (`w`)
- Filter by resource kind (`k`)
- Color-coded by event type

### Pod Details (Enter on pod)
Detailed information about a pod including:
- Metadata (labels, annotations)
- Container information
- Resource requests/limits
- Recent events

### Log Viewer (`l`)
View pod logs with streaming support:
- Follow mode for real-time logs
- Multi-container support
- Timestamp toggle
- Search with highlighting
- ANSI color preservation

### SSH Hosts View (`9`)
Connect to K3s nodes via SSH to run crictl commands:
- View containers on the node
- Inspect container logs
- See node system information

## SSH Authentication

k4s supports two methods for SSH authentication:

1. **SSH Agent** (Recommended) - If you have ssh-agent running with keys loaded:
   ```bash
   eval $(ssh-agent)
   ssh-add ~/.ssh/id_rsa
   ```

2. **Passphrase Prompt** - If your key requires a passphrase and ssh-agent isn't available, k4s will prompt for it.

## Logs

Debug logs are written to `~/.k4s/k4s.log`. Useful for troubleshooting connection issues.

## Development

### Prerequisites

- Go 1.21 or later
- Access to a K3s/Kubernetes cluster

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run tests
make test

# Run linters
make lint

# Format code
make fmt
```

### Project Structure

```
k4s/
├── cmd/k4s/          # Application entry point
├── internal/
│   ├── adapter/
│   │   ├── config/   # Configuration loading
│   │   ├── k8s/      # Kubernetes client
│   │   ├── ssh/      # SSH client & crictl
│   │   └── tui/      # Terminal UI components
│   ├── domain/       # Domain models
│   └── logger/       # Logging
├── .claude/          # Development plans & skills
├── Makefile          # Build automation
└── README.md
```

## Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- [k9s](https://k9scli.io/) - Inspiration for this project
- [Charm](https://charm.sh/) - Amazing TUI libraries (bubbletea, bubbles, lipgloss)
- [client-go](https://github.com/kubernetes/client-go) - Kubernetes Go client

---

Made with :heart: for the K3s community
