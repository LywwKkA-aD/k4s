# k4s

A fast, opinionated TUI for Kubernetes (k8s & k3s) — written in Go on top of
[Bubble Tea](https://github.com/charmbracelet/bubbletea).

> Hybrid TUI **and** kubectl companion. Every keystroke shows its `kubectl`
> equivalent in the footer, so you build CLI muscle memory while you work.

<!-- demo placeholder — replaced by docs/demo.gif once recorded -->
<p align="center">
  <img src="docs/demo.gif" alt="k4s demo" width="780">
</p>

## Highlights

- **Lists for the basics** — pods, namespaces, deployments, services
- **Describe** — kubectl-style sections + events, scrollable
- **Multi-pod log streaming** — per-pod FNV colour tag, search (`/`),
  smart auto-follow, two-step clear, 5000-line buffer
- **Exec** — `e` drops you into a pod shell via `kubectl exec`
- **Top** — `kubectl top pods/nodes` straight from the metrics API
- **Live filter** — `/` substring filter inside any list view
- **Context switcher** — `:ctx` to jump between kubeconfig contexts
- **Watch by default** — `w` toggles 5s auto-refresh, footer shows `--watch`
- **Footer hints** — every shortcut prints the equivalent `kubectl` command
- **Centred help popup** — `?` shows global + view-specific bindings

## Quickstart

Requires Go 1.26+, Docker, and `kubectl` on PATH.

```bash
# bring up local k3s in docker, seed demo workloads, write .kube/config
make demo

# run k4s against that cluster
KUBECONFIG=$(pwd)/.kube/config make run
```

`make help` lists every target. `make k3s-clean` wipes the local cluster.

## Keys

| Global  | Action                              |
| ------- | ----------------------------------- |
| `?`     | help popup                          |
| `:`     | command bar (`:pods`, `:ns`, `:ctx`, `:top`, ...) |
| `/`     | filter the current list             |
| `q`     | go home, then quit on the dashboard |
| `Esc`   | back                                |
| `w`     | toggle 5s auto-refresh              |
| `Ctrl+C`| force quit                          |

| In a pod row | Action                                |
| ------------ | ------------------------------------- |
| `Enter`      | describe                              |
| `l`          | tail logs (with container picker)     |
| `e`          | `kubectl exec -it -- sh`              |

| In logs view | Action                                |
| ------------ | ------------------------------------- |
| `/`          | search                                |
| `n` / `N`    | next / previous match                 |
| `f`          | toggle auto-follow                    |
| `t`          | toggle full pod names vs compact tag  |
| `c`          | clear search → clear buffer           |

## Demo workloads

`make seed` applies the manifests in `deploy/seed/`:

- `k4s-demo` and `k4s-broken` namespaces
- `nginx` Deployment (3 replicas) + Service + ConfigMap
- `web-with-sidecar` — pod with two containers for container-switching tests
- `log-spammer` — two pods, one log line per second, ideal for multi-log streaming
- `crash-loop` — exercises the `CrashLoopBackOff` rendering
- a Secret and a CronJob (every 2 minutes) for read-only browsing

## Quality gate

All linters and vulnerability scanners are pinned through the `tool` directive
in `go.mod` — no global installs needed.

```bash
make fmt    # gofmt + goimports (via golangci-lint)
make lint   # golangci-lint v2 — errcheck, staticcheck, gosec, revive, gocyclo, ...
make vuln   # govulncheck — known CVEs in the dependency graph
make ci     # vet + lint + test + vuln
```

`gosec` runs as part of `make lint` and covers the OWASP-aligned checks
(hardcoded credentials, weak crypto, integer overflows, path traversal, ...).

## Project layout

```
cmd/k4s/                entry point
internal/
  tui/                  Bubble Tea root, views, styles, keymap, command bar
    views/{dashboard,pods,namespaces,deployments,services,
           contexts,top,describe,logs}/
    {filter,command,help,...}
  k8s/                  client-go wrapper (pods/deployments/services/logs/metrics/...)
  actions/              Action abstraction with kubectl-equivalent strings
deploy/seed/            demo manifests
docker-compose.yml      single-node k3s for local development
Makefile                build / test / k3s / quality targets
```

## Status

MVP complete. Watch this space for resource actions (delete / scale /
rollout restart), a YAML view, ConfigMap & Secret browsing, and an events
view.

## License

MIT — see [LICENSE](./LICENSE).
