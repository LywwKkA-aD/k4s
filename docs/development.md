# Development

## Prerequisites

- Go 1.21 or later
- Access to a K3s/Kubernetes cluster

## Building

```bash
# Build for current platform
make build

# Build for all platforms (linux/darwin, amd64/arm64)
make build-all

# Install to /usr/local/bin
make install

# Run tests
make test

# Run tests with coverage
make test-cover

# Run linters
make lint

# Format code
make fmt

# Clean build artifacts
make clean
```

## Project Structure

```
k4s/
├── cmd/k4s/              # Application entry point
│   └── main.go
├── internal/
│   ├── adapter/
│   │   ├── config/       # Configuration loading
│   │   ├── k8s/          # Kubernetes client
│   │   ├── ssh/          # SSH client & crictl
│   │   └── tui/          # Terminal UI components
│   ├── domain/           # Domain models
│   └── logger/           # Logging
├── docs/                 # Documentation
├── Makefile              # Build automation
└── README.md
```

## Architecture

k4s follows **Hexagonal Architecture** (Ports & Adapters):

- **Domain Layer** (`internal/domain/`) - Pure data models with no external dependencies
- **Adapter Layer** (`internal/adapter/`) - Implementations for external systems
  - `config/` - YAML configuration via Viper
  - `k8s/` - Kubernetes API via client-go
  - `ssh/` - SSH connections via golang.org/x/crypto
  - `tui/` - Terminal UI via Bubbletea

## Tech Stack

| Component | Library |
|-----------|---------|
| TUI Framework | [Bubbletea](https://github.com/charmbracelet/bubbletea) |
| UI Components | [Bubbles](https://github.com/charmbracelet/bubbles) |
| Styling | [Lipgloss](https://github.com/charmbracelet/lipgloss) |
| Kubernetes | [client-go](https://github.com/kubernetes/client-go) |
| Configuration | [Viper](https://github.com/spf13/viper) |
| SSH | [golang.org/x/crypto/ssh](https://pkg.go.dev/golang.org/x/crypto/ssh) |

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Release Process

```bash
# Create release archives
make release

# Output in dist/
# - k4s-vX.X.X-linux-amd64.tar.gz
# - k4s-vX.X.X-linux-arm64.tar.gz
# - k4s-vX.X.X-darwin-amd64.tar.gz
# - k4s-vX.X.X-darwin-arm64.tar.gz
```
