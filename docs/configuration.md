# Configuration

k4s uses a YAML configuration file at `~/.k4s/config.yaml`. A default configuration is created on first run.

## Example Configuration

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

## Kubeconfig Options

| Field | Description |
|-------|-------------|
| `name` | Display name for the cluster |
| `path` | Path to kubeconfig file (supports `~`) |
| `default` | Set to `true` for auto-selection on startup |

## SSH Host Options

| Field | Description |
|-------|-------------|
| `name` | Display name for the node |
| `host` | Hostname or IP address |
| `user` | SSH username |
| `key_path` | Path to SSH private key (supports `~`) |
| `port` | SSH port (default: 22) |

## File Locations

| Path | Description |
|------|-------------|
| `~/.k4s/config.yaml` | Main configuration file |
| `~/.k4s/logs/` | Debug logs directory |
| `~/.k4s/logs/k4s-YYYY-MM-DD.log` | Daily log files |
