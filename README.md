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
- **Multi-pod log streaming** — per-pod colour tag, search (`/`),
  smart auto-follow, two-step clear, 5000-line buffer. Drain-batch
  render means `--tail=5000` snaps to the bottom instantly *(v1.1.0)*.
  Pod colours come from a hue-separated palette, so neighbouring pods
  never share a hue *(v1.1.3)*
- **Soft-wrap + horizontal scroll** — `w` in logs view toggles wrap,
  `←`/`→` pan when wrap is off *(v1.1.0)*
- **Port-forward, persisted** — `f` in services / pods / deployments
  opens an SPDY tunnel; intents are saved to
  `~/.local/state/k4s/portforwards.json` so the next launch can
  revive every forward with one keystroke. `:pf` / `:forwards` lists
  active sessions *(v1.1.0)*
- **Logs from `:svc` too** — `l` on a service tails every backing pod,
  same flow as `:deploy` *(v1.1.1)*
- **Time-merged multi-pod history** — `--tail` bursts from several pods
  are sorted by timestamp instead of arriving pod-by-pod *(v1.1.2)*
- **Exec** — `e` drops you into a pod shell via `kubectl exec`
- **Top** — `kubectl top pods/nodes` straight from the metrics API
- **Live filter** — `/` substring filter in the pods, namespaces,
  deployments and services lists
- **Context switcher** — `:ctx` to jump between kubeconfig contexts
- **Watch by default** — `w` toggles 5s auto-refresh on the dashboard,
  in `:top` and in the lists; the footer shows `--watch`
- **Readable on any theme** — dark / light background detection,
  `K4S_THEME` and `NO_COLOR` *(v1.1.5)*
- **Footer hints** — every shortcut prints the equivalent `kubectl` command
- **Centred help popup** — `?` shows global + view-specific bindings

## Install

### Prebuilt binary

Every [release](https://github.com/LywwKkA-aD/k4s/releases/latest) from
v1.1.5 on carries an archive per platform and a `checksums.txt` (SHA-256).
Each archive holds the `k4s` binary, `LICENSE` and `README.md`.

| Platform                                   | Archive                                    |
| ------------------------------------------ | ------------------------------------------ |
| Linux amd64 / arm64 / armv7 (Raspberry Pi) | `k4s_<version>_linux_<amd64\|arm64\|armv7>.tar.gz` |
| macOS Intel / Apple silicon                | `k4s_<version>_darwin_<amd64\|arm64>.tar.gz` |
| Windows amd64                              | `k4s_<version>_windows_amd64.zip`          |

`<version>` has no leading `v`, e.g. `k4s_1.1.5_linux_arm64.tar.gz`.

```bash
V=1.1.5 TARGET=linux_arm64
curl -fLO "https://github.com/LywwKkA-aD/k4s/releases/download/v${V}/k4s_${V}_${TARGET}.tar.gz"
curl -fLO "https://github.com/LywwKkA-aD/k4s/releases/download/v${V}/checksums.txt"
sha256sum --ignore-missing -c checksums.txt   # macOS: shasum -a 256 --ignore-missing -c checksums.txt
tar xzf "k4s_${V}_${TARGET}.tar.gz" k4s
sudo install k4s /usr/local/bin/
k4s --version
```

- **Windows:** unzip, then compare
  `(Get-FileHash k4s_1.1.5_windows_amd64.zip -Algorithm SHA256).Hash` with
  that file's line in `checksums.txt`.
- **macOS:** the binaries are not notarized. If a browser downloaded the
  archive, clear the quarantine flag with `xattr -d com.apple.quarantine k4s`.

### With Go

```bash
go install github.com/LywwKkA-aD/k4s/cmd/k4s@latest
```

Requires Go 1.26+. `kubectl` on `PATH` is needed only for exec (`e`).

## Usage

```bash
k4s              # kubeconfig from $KUBECONFIG, else ~/.kube/config
k4s --version    # print the version and exit
k4s --help       # list the flags and exit
```

k4s never prints over the TUI. client-go errors and Kubernetes API warnings
go to `$XDG_STATE_HOME/k4s/k4s.log` (default `~/.local/state/k4s/k4s.log`);
if anything was logged, k4s prints the count and the path when it exits.

Contexts that log in through a credential plugin (EKS, GKE, OIDC/kubelogin)
are contacted once before the TUI starts, so an SSO or device-code prompt
shows up on the plain terminal. Plugin output while the TUI runs is kept in
memory and printed after exit; it is never written to the log file.

## Themes & accessibility

At startup k4s detects whether the terminal background is dark or light and
picks its colours to match. Override it with `K4S_THEME`:

| `K4S_THEME`      | Colours                                         |
| ---------------- | ----------------------------------------------- |
| `auto` (default) | detected from the terminal background           |
| `dark`           | tuned for dark backgrounds                      |
| `light`          | tuned for light backgrounds                     |
| `mono`           | none: bold, underline and reverse video only    |

Setting [`NO_COLOR`](https://no-color.org) gives `mono`. Any explicit
`K4S_THEME` skips the background detection.

On Windows the background cannot be detected and dark is assumed; on a light
theme set `K4S_THEME=light` (PowerShell: `$env:K4S_THEME = "light"`).

## Quickstart (local k3s)

To try k4s against a throwaway cluster, or to hack on it: requires Go 1.26+,
Docker, and `kubectl` on PATH.

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
| `?`      | help popup (global + current view)                         |
| `:`      | command bar (`:pods`, `:ns`, `:ctx`, `:top`, `:pf`, ...)   |
| `q`      | go home, then quit on the dashboard                        |
| `Esc`    | back                                                       |
| `Ctrl+C` | force quit                                                 |

| In the pods, namespaces, deployments, services lists | Action                                                          |
| ---------------------------------------------------- | --------------------------------------------------------------- |
| `/`                                                  | filter by substring                                             |
| `w`                                                  | toggle 5s auto-refresh (also on the dashboard and in `:top`)    |
| `r`                                                  | refresh now (also on the dashboard, in `:top`, `:ctx`, describe) |

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

| Other views                  | Action                          |
| ---------------------------- | ------------------------------- |
| `Enter` in `:ns`             | switch to that namespace        |
| `Enter` in `:ctx`            | switch to that context          |
| `n` in `:top`                | toggle pods / nodes             |

In the logs view `/` searches and `w` toggles wrap:

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

Linters and the vulnerability scanner are pinned in a separate module,
`tools/go.mod` (run as `go tool -modfile=tools/go.mod <tool>`), so tool
upgrades never move the app's dependencies — no global installs needed. The
tool targets run on the toolchain pinned by `go.mod`; the first run may
download it (`GOTOOLCHAIN=local make lint` to opt out).

```bash
make fmt         # gofmt + goimports (via golangci-lint)
make lint        # golangci-lint v2 — errcheck, staticcheck, gosec, revive, gocyclo, ...
make vuln        # govulncheck — known CVEs in the dependency graph
make tidy-check  # go.mod / go.sum of both modules are tidy
make ci          # tidy-check + vet + lint + race tests + vuln
```

GitHub Actions runs `make ci` and cross-builds (linux amd64/arm64/armv7,
darwin, windows) on every push and pull request, plus a weekly scheduled run
to catch new advisories. GitHub disables schedules on public repos after 60
days without activity; re-enable with `gh workflow enable ci`.

`gosec` runs as part of `make lint` and covers the OWASP-aligned checks
(hardcoded credentials, weak crypto, integer overflows, path traversal, ...).

## Releases

Pushing a `v*` tag runs `.github/workflows/release.yml`: GoReleaser
(`.goreleaser.yaml`) builds the six archives and `checksums.txt` and uploads
them to that tag's GitHub release, keeping the hand-written release notes.

```bash
make build       # bin/k4s, version stamped from `git describe`
make snapshot    # every release archive into dist/, nothing published
```

`make snapshot` fetches GoReleaser with `go run`, so its first run needs
network access.

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
  version/              build version (ldflags stamp, else module version)
deploy/seed/            demo manifests
docker-compose.yml      single-node k3s for local development
Makefile                build / test / k3s / quality / release targets
.goreleaser.yaml        release archives + checksums
```

## Status

Release notes for every version are on the
[releases page](https://github.com/LywwKkA-aD/k4s/releases);
[CHANGELOG.md](./CHANGELOG.md) has the short version.

Watch this space for resource actions (delete / scale / rollout restart),
a YAML view, ConfigMap & Secret browsing, an events view, and a Homebrew
tap. Roadmap on <https://k4scli.io/changelog/>.

## License

MIT — see [LICENSE](./LICENSE).
