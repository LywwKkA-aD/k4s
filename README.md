# k4s

A fast, opinionated TUI for Kubernetes (k8s & k3s), written in Go.

## Why k4s and not k9s?

Two ideas that set k4s apart:

1. **Hybrid TUI / kubectl companion.** Use it as a full TUI, or as a polished wrapper that surfaces the `kubectl` equivalent of every keystroke — great for learning the CLI by muscle memory.
2. **Full management, not just inspection.** Lists, describe, edit, delete, multi-pod log streaming, exec — all in one place, built around an `Action` abstraction so each operation is auditable and learnable.

## MVP scope

- Pods / namespaces / services / deployments — list + describe
- Container switching inside a pod
- Multi-pod log streaming with per-pod color tagging
- `exec` into a pod
- Footer hints showing the `kubectl` equivalent of every shortcut

## Quickstart

Requires Go 1.23+, Docker, and `kubectl` on PATH.

```bash
# bring up local k3s in docker and write .kube/config
make k3s-up

# run k4s against that cluster
KUBECONFIG=$(pwd)/.kube/config make run
```

`make help` lists every target.

## Project layout

```
cmd/k4s/             entry point
internal/
  tui/               Bubble Tea models, views, styles, keymap
  k8s/               client-go wrapper
  actions/           Action abstraction (kubectl-equivalent hints)
  config/            application config (placeholder)
docker-compose.yml   single-node k3s for local testing
Makefile             build / test / k3s tasks
```

## Status

Early skeleton. See the issue tracker for milestones.

## License

MIT — see [LICENSE](./LICENSE).
