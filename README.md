# k4s

[![release](https://img.shields.io/github/v/release/LywwKkA-aD/k4s?color=B4BEFE&style=flat)](https://github.com/LywwKkA-aD/k4s/releases/latest)
[![license](https://img.shields.io/github/license/LywwKkA-aD/k4s?color=B4BEFE&style=flat)](./LICENSE)
[![site](https://img.shields.io/badge/site-k4scli.io-B4BEFE?style=flat)](https://k4scli.io)

A fast, opinionated TUI for Kubernetes (k8s & k3s) — written in Go on top of
[Bubble Tea](https://github.com/charmbracelet/bubbletea).

> Hybrid TUI **and** kubectl companion. Every keystroke shows its `kubectl`
> equivalent in the footer, so you build CLI muscle memory while you work.

<!-- demo.gif: pre-v1.1 recording — re-record once port-forward and the
     services 'l' shortcut have a tape of their own. -->
<p align="center">
  <img src="docs/demo.gif" alt="k4s demo" width="780">
</p>

## Highlights

- **Lists for the basics** — pods, namespaces, deployments, services
- **Describe** — kubectl-style sections + events, scrollable
- **Multi-pod log streaming** — per-pod FNV colour tag, search (`/`),
  smart auto-follow, two-step clear, 5000-line buffer. Drain-batch
  render means `--tail=5000` snaps to the bottom instantly *(v1.1.0)*
- **Soft-wrap + horizontal scroll** — `w` in logs view toggles wrap,
  `←`/`→` pan when wrap is off *(v1.1.0)*
- **Port-forward, persisted** — `f` in services / pods / deployments
  opens an SPDY tunnel; intents are saved to
  `~/.local/state/k4s/portforwards.json` so the next launch can
  revive every forward with one keystroke. `:pf` / `:forwards` lists
  active sessions *(v1.1.0)*
- **Logs from `:svc` too** — `l` on a service tails every backing pod,
  same flow as `:deploy` *(v1.1.1)*
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

| Global   | Action                                                     |
| -------- | ---------------------------------------------------------- |
| `?`      | help popup                                                 |
| `:`      | command bar (`:pods`, `:ns`, `:ctx`, `:top`, `:pf`, ...)   |
| `/`      | filter the current list                                    |
| `q`      | go home, then quit on the dashboard                        |
| `Esc`    | back                                                       |
| `w`      | toggle 5s auto-refresh                                     |
| `Ctrl+C` | force quit                                                 |

| In a pod row    | Action                                |
| --------------- | ------------------------------------- |
| `Enter`         | describe                              |
| `l`             | tail logs (with container picker)     |
| `e`             | `kubectl exec -it -- sh`              |
| `f`             | port-forward to the pod               |

| In a service row | Action                                                       |
| ---------------- | ------------------------------------------------------------ |
| `Enter`          | describe                                                     |
| `l`              | tail logs of every backing pod *(v1.1.1)*                    |
| `f`              | port-forward to the service (default `local = remote + 8000` if remote < 1024) |

| In a deployment row | Action                                |
| ------------------- | ------------------------------------- |
| `Enter`             | describe                              |
| `l`                 | tail logs of every replica            |
| `f`                 | port-forward to any backing pod       |

| In logs view  | Action                                          |
| ------------- | ----------------------------------------------- |
| `/`           | search                                          |
| `n` / `N`     | next / previous match                           |
| `f`           | toggle auto-follow                              |
| `t`           | toggle full pod names vs compact tag            |
| `w`           | toggle soft-wrap *(v1.1.0)*                     |
| `←` / `→`     | horizontal scroll (no-wrap mode) *(v1.1.0)*     |
| `c`           | clear search → clear buffer                     |

| In forwards view (`:pf`) | Action                          |
| ------------------------ | ------------------------------- |
| `Enter`                  | start a stopped forward         |
| `s`                      | stop a running forward          |
| `r`                      | restart                         |
| `d`                      | delete from state               |

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
           contexts,top,describe,logs,forwards}/
    {filter,command,help,...}
  k8s/                  client-go wrapper (pods/deployments/services/logs/
                        metrics/portforward/...)
  forwards/             port-forward state file + Manager (intents persist
                        across launches; SPDY sessions are in-process)
  actions/              Action abstraction with kubectl-equivalent strings
deploy/seed/            demo manifests
docker-compose.yml      single-node k3s for local development
Makefile                build / test / k3s / quality targets
```

## Status

Latest: [**v1.1.1**](https://github.com/LywwKkA-aD/k4s/releases/tag/v1.1.1) —
services `l` tails all backing pods. v1.1.0 added in-process port-forwarding
with persisted intents, drain-batch logs render, soft-wrap and horizontal
scroll. v1.0.0 was the initial kubectl-aware TUI.

Watch this space for resource actions (delete / scale / rollout restart),
a YAML view, ConfigMap & Secret browsing, an events view, and a Homebrew
tap. Roadmap on <https://k4scli.io/changelog/>.

## License

MIT — see [LICENSE](./LICENSE).
