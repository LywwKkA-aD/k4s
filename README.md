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

## Local demo workloads

After `make k3s-up`, populate the cluster with realistic-looking demo resources:

```bash
make seed       # apply deploy/seed/*.yaml
make seed-down  # remove them
make demo       # = k3s-up + seed
```

The seed includes:

- `k4s-demo` and `k4s-broken` namespaces
- `nginx` Deployment (3 replicas) + Service + ConfigMap
- `web-with-sidecar` — Pod with two containers (web + tailing sidecar) for container-switching tests
- `log-spammer` Deployment (2 replicas), each pod prints once per second — handy for multi-log streaming tests
- `crash-loop` Pod that intentionally fails — exercises CrashLoopBackOff rendering
- A demo Secret and a CronJob (every 2 minutes) for read-only browsing

## Quality gate

All linters and security scanners are pinned through the `tool` directive in
`go.mod` — no global installs needed.

```bash
make fmt    # auto-format (gofmt + goimports)
make lint   # golangci-lint v2: errcheck, staticcheck, gosec, revive, gocyclo, ...
make vuln   # govulncheck — scan deps for known CVEs
make ci     # vet + lint + test + vuln
```

`gosec` runs as part of `make lint` and covers hardcoded credentials, weak
crypto, integer overflows, path traversal and other OWASP-aligned checks.

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
