# k4s

A lightweight Terminal UI for K3s/Kubernetes cluster management, built on [Charm](https://charm.sh/).

![k4s demo](https://img.shields.io/badge/version-0.2.0-blue)
![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-green)

## Features

- **Real-time Monitoring** - Live pods, deployments, services, events with auto-refresh
- **Resource Metrics** - CPU/Memory usage (requires metrics-server)
- **Streaming Logs** - Follow logs with search & highlighting
- **SSH Integration** - Connect to nodes and inspect containers via crictl
- **Keyboard-driven** - Vim-style navigation

## Quick Start

```bash
# From source
git clone https://github.com/LywwKkA-aD/k4s.git
cd k4s && make install

# Or download from releases
# https://github.com/LywwKkA-aD/k4s/releases
```

Configuration is stored at `~/.k4s/config.yaml` (auto-created on first run).

## Keybindings

| Key | Action |
|-----|--------|
| `?` | Help |
| `1-5` | Switch views (Namespaces/Pods/Deployments/Services/Events) |
| `9` | SSH Hosts |
| `j/k` | Navigate |
| `Enter` | Select |
| `l` | Logs |
| `q` | Quit |

See full documentation in [docs/](docs/).

## Contributing

Contributions welcome! Please check our [issues](https://github.com/LywwKkA-aD/k4s/issues) or open a new one.

## License

MIT License - see [LICENSE](LICENSE)

---

[![Stargazers repo roster for @LywwKkA-aD/k4s](https://reporoster.com/stars/LywwKkA-aD/k4s)](https://github.com/LywwKkA-aD/k4s/stargazers)
