# SSH Integration

k4s can connect to K3s/Kubernetes nodes via SSH to run crictl commands for container runtime inspection.

## Configuration

Add SSH hosts to your `~/.k4s/config.yaml`:

```yaml
ssh_hosts:
  - name: "k3s-node-1"
    host: "192.168.1.100"
    user: "admin"
    key_path: "~/.ssh/id_rsa"
    port: 22
```

## Authentication Methods

### 1. SSH Agent (Recommended)

If you have ssh-agent running with keys loaded:

```bash
eval $(ssh-agent)
ssh-add ~/.ssh/id_rsa
```

k4s will automatically use the agent for authentication.

### 2. Passphrase Prompt

If your key requires a passphrase and ssh-agent isn't available, k4s will prompt for the passphrase.

## Usage

1. Press `9` to open SSH Hosts view
2. Select a node and press `Enter`
3. k4s connects and runs crictl to list containers
4. Navigate containers and view logs

## Requirements

- SSH access to the node
- `crictl` installed on the node
- Appropriate permissions to run crictl (usually root or docker group)

## crictl Features

Once connected to a node:

| Key | Action |
|-----|--------|
| `Enter` | View container details |
| `l` | View container logs |
| `Esc` | Disconnect and go back |

## Troubleshooting

**Connection refused:**
- Verify the host and port are correct
- Check if SSH service is running on the node

**Permission denied:**
- Verify the username and key path
- Check if the key is added to authorized_keys on the node

**crictl errors:**
- Ensure crictl is installed on the node
- User may need sudo/root access for crictl

Debug logs are written to `~/.k4s/logs/` for troubleshooting.
