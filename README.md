# k4s

[![release](https://img.shields.io/github/v/release/LywwKkA-aD/k4s?color=B4BEFE&style=flat)](https://github.com/LywwKkA-aD/k4s/releases/latest)
[![license](https://img.shields.io/github/license/LywwKkA-aD/k4s?color=B4BEFE&style=flat)](./LICENSE)
[![site](https://img.shields.io/badge/site-k4scli.io-B4BEFE?style=flat)](https://k4scli.io)

A fast, opinionated TUI for Kubernetes (k8s & k3s) — written in Go on top of
[Bubble Tea](https://github.com/charmbracelet/bubbletea).

> Hybrid TUI **and** kubectl companion. Every keystroke shows its `kubectl`
> equivalent in the footer, so you build CLI muscle memory while you work.

<!-- demo.gif: pre-v1.1 recording, from before the dashboard — re-record
     with docs/demo.tape once the 1.2 screens have a tape of their own. -->
<p align="center">
  <img src="docs/demo.gif" alt="k4s demo" width="780">
</p>

## Highlights

- **kubectl companion** — the footer always shows the exact `kubectl`
  command behind the screen, or behind the panel or row under the cursor,
  with `--context` and `--kubeconfig` added whenever a plain `kubectl` in
  your shell would reach another cluster. `y` copies it, exec and describe
  run that very argv, and on exit k4s prints the session's commands so they
  stay in your scrollback. `≈` marks the few commands that do a little more
  or less than k4s
- **Dashboard** — the home screen answers three questions in order: is the
  cluster OK (a verdict line), what do I run next (up to five `▸ next`
  suggestions, each a read-only kubectl command), and where is the problem
  (seven numbered panels: cluster, nodes, pods, workloads, events, k3s, top
  pods). Every panel and row maps to one kubectl command; Enter opens the
  matching k4s view
- **Lists** — pods, deployments, services, namespaces, events and nodes,
  with a `/` filter, auto-refresh, and Esc back to the list as you left it
- **Describe and YAML** — `kubectl describe` through your own kubectl (a
  built-in describer when there is none) and `o` for `kubectl get -o yaml`,
  both searchable with `/`
- **Logs you can trust** — every pod of a deployment or a service in one
  stream, with per-pod colours; the pods of a rollout join on their own,
  every stream that ends says why, `p` shows the previous instance, and
  search is smart-case. Soft-wrap (`w`) and horizontal scroll (`←`/`→`)
- **Exec, port-forward, top** — `e` opens a shell in a pod (`cmd.exe` in
  Windows pods), `f` a port-forward that the next launch can revive, and
  `:top` shows pods and nodes from the metrics API
- **Any cluster, any RBAC** — a list forbidden across all namespaces falls
  back to the context's namespace, and every error says in one line what
  went wrong and which `kubectl auth can-i` asks the same
- **Best on k3s** — the header names the distribution and release, the
  dashboard checks k3s's bundled components, and on a k3s node k4s finds
  `/etc/rancher/k3s/k3s.yaml` and `k3s kubectl` by itself
- **Readable on any theme** — dark / light background detection, a
  high-contrast theme, `K4S_THEME` and `NO_COLOR`; every state is a glyph
  and a word, never colour alone
- **Safe to point at any cluster** — text from the cluster is made inert
  before it reaches your terminal, and `y` refuses a command in which a
  name from the cluster would set `--server` or `--kubeconfig`

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

Requires Go 1.26+.

`kubectl` on `PATH` (or `k3s kubectl` on a k3s node) is needed for exec
(`e`) and for the terminal login of a credential plugin. Describe runs it
too when it is there; without it k4s describes pods, deployments and
services with a built-in describer.

## Usage

```bash
k4s                          # current context, starting in its namespace
k4s --context staging -n web # another context and namespace
k4s -A                       # all namespaces
k4s --kubeconfig ./lab.yaml  # a kubeconfig of your choice
k4s --theme high-contrast    # text at 7:1 or better
k4s --no-recap               # print no kubectl commands on exit
k4s --version                # print the version and exit
k4s --help                   # list the flags and exit
```

| Flag                       | Meaning                                                              |
| -------------------------- | -------------------------------------------------------------------- |
| `--kubeconfig <file>`      | kubeconfig to use instead of the lookup below                        |
| `--context <name>`         | context to use instead of the kubeconfig's current-context           |
| `-n`, `--namespace <name>` | namespace to start in                                                |
| `-A`, `--all-namespaces`   | start in all namespaces                                              |
| `--theme <mode>`           | `auto`, `dark`, `light`, `mono` or `high-contrast`; wins over `K4S_THEME` and `NO_COLOR` |
| `--no-preflight`           | do not run the credential plugin before the TUI starts               |
| `--no-recap`               | do not print the session's kubectl commands on exit                  |
| `-h`, `--help`             | show the flags and exit                                              |
| `--version`                | print the version and exit                                           |

| Environment        | Meaning                                                              |
| ------------------ | -------------------------------------------------------------------- |
| `KUBECONFIG`       | kubeconfig file list, as kubectl reads it                            |
| `K4S_THEME`        | `auto` (default), `dark`, `light`, `mono` or `high-contrast`         |
| `K4S_NO_PREFLIGHT` | anything but `0`, `false`, `no` or `off` acts as `--no-preflight`    |
| `K4S_NO_RECAP`     | anything but `0`, `false`, `no` or `off` acts as `--no-recap`        |
| `NO_COLOR`         | set: no colour, as `K4S_THEME=mono`                                  |
| `XDG_STATE_HOME`   | where `k4s/k4s.log` and `k4s/portforwards.json` live when it is an absolute path (default `~/.local/state`) |

A `--kubeconfig` or `--context` that cannot be loaded stops k4s before the
TUI starts. Without `--kubeconfig`, k4s looks in this order:

1. `$KUBECONFIG` (a list, as kubectl reads it);
2. `~/.kube/config`;
3. on Linux, the k3s server's own `/etc/rancher/k3s/k3s.yaml`, so k4s works
   on a k3s node without setup. k3s writes that file for root only; when it
   is not readable k4s prints how to make a copy for your user.

k4s starts in the namespace the context sets, as kubectl does, and in all
namespaces when it sets none; `-n` and `-A` override it. In the TUI,
`:ns <name>` switches namespace and `:ns -A` shows all of them (`all` is a
valid namespace name, so it means that namespace).

Footer commands always reach the cluster k4s shows when pasted into the
shell k4s was started from: `--context` is added when you passed
`--context` or k4s talks to another context than the kubeconfig's
current-context (after `:ctx`, say), and `--kubeconfig` when k4s reads a
file a plain `kubectl` would not (an explicit `--kubeconfig`, the k3s
fallback). `y`
copies the command through the terminal (OSC 52), which also works over
SSH; inside tmux it needs `set -g set-clipboard on`. `y` copies nothing,
and says why, when a word in the command would set a session flag such as
`--server` (a hostile object name can carry one) or holds control
characters.

When k4s exits it prints the kubectl commands the session led to (the
views you opened, describe, logs, exec and port-forward), each once, the 15
most recently used, oldest first, as the footer showed them. They stay in
your shell's scrollback after the TUI has gone; `--no-recap` or
`K4S_NO_RECAP=1` turns this off.

k4s never prints over the TUI. client-go errors and Kubernetes API warnings
go to `$XDG_STATE_HOME/k4s/k4s.log` (default `~/.local/state/k4s/k4s.log`);
if anything was logged, k4s prints the count and the path when it exits.

Contexts that log in through a credential plugin (EKS, GKE, OIDC/kubelogin)
are contacted once before the TUI starts, so an SSO or device-code prompt
shows up on the plain terminal; it gives up after 30 s. Plugin output while
the TUI runs is kept in memory and printed after exit; it is never written
to the log file. When a plugin needs a login while k4s owns the terminal
(after `:ctx`, or after a preflight that failed for want of one), k4s asks
whether to log in; Enter runs `kubectl version` for that context on the
plain terminal and comes back.

## Commands

`:` opens the command bar in the footer. Commands ignore case.

| Command        | Aliases                                       | Shows                                      |
| -------------- | --------------------------------------------- | ------------------------------------------ |
| `:dashboard`   | `:home`                                       | the dashboard                              |
| `:pods`        | `:po`, `:pod`                                 | `kubectl get pods`                         |
| `:deployments` | `:deploy`, `:deployment`, `:dp`               | `kubectl get deployments`                  |
| `:services`    | `:svc`, `:service`                            | `kubectl get services`                     |
| `:namespaces`  | `:ns`                                         | `kubectl get namespaces`                   |
| `:events`      | `:ev`, `:event`                               | `kubectl events`                           |
| `:nodes`       | `:no`, `:node`                                | `kubectl get nodes -o wide`                |
| `:top`         | `:metrics`                                    | `kubectl top pods` (`n` for nodes)         |
| `:contexts`    | `:ctx`, `:context`                            | `kubectl config get-contexts`              |
| `:forwards`    | `:pf`, `:forward`, `:portforward`, `:port-forward` | the port-forwards of this cluster     |

`:pods`, `:deploy`, `:svc`, `:top` and `:events` take a namespace as
kubectl spells it:

```
:pods demo        :pods -n demo        :pods -A        :pods --all-namespaces
```

From another view the list opens in that namespace, one Esc away; on the
list itself it switches in place, keeping the filter, watch and mode.
`:ns <name>` does the same for the current list (from any other view it
opens pods there) and `:ns -A` shows every namespace. The name is checked
as a namespace name and never looked up, since RBAC may forbid reading
namespaces. `:nodes` and the other commands take no namespace.

## Keys

The footer's first row lists the keys of the screen you are on, and `?`
lists them all. While a filter, a search or a prompt has the keyboard,
every key goes to it: `q`, `y` and `o` are just letters there.

| Global   | Action                                                              |
| -------- | ------------------------------------------------------------------- |
| `q`      | go home to the dashboard; on the dashboard, quit                    |
| `ctrl+c` | quit, from anywhere                                                 |
| `esc`    | back: first what the view uses it for (a kept filter, a search, a zoom), then the previous view as you left it |
| `:`      | command bar (see [Commands](#commands))                             |
| `?`      | help: global keys, the view's keys, the dashboard map, the commands; `?`, `esc` or `q` close it |
| `y`      | copy the footer's kubectl command (OSC 52)                          |
| `o`      | YAML of the object the view shows or has under the cursor           |

### Dashboard

| Key                   | Action                                                       |
| --------------------- | ------------------------------------------------------------ |
| `tab` / `shift+tab`   | next / previous panel                                        |
| `0`–`7`               | jump to a panel (`0` is the banner); one the terminal is too small for opens zoomed |
| `←` / `→`             | neighbouring panel                                           |
| `↑` / `↓` (`k` / `j`) | rows of the focused panel; in the banner, the `▸ next` suggestions |
| `enter`               | on a panel, its k4s view (`:pods`, `:deploy`, `:events` with only the warnings, ...); on a row, describe its object or open its list; on a suggestion, what it says; on the verdict, the most severe panel |
| `l`                   | logs of a pod row; on a crash loop suggestion, the previous instance's |
| `z`                   | zoom the focused panel to the whole screen, and back         |
| `s`                   | in panel 7, sort the top pods by CPU or memory               |
| `r`                   | refresh every source now                                     |
| `w`                   | watch on or off; off polls nothing                           |
| `esc`                 | leave the zoom, then the row, then the panel                 |
| `o` / `y`             | YAML of the row's object / copy the focused command          |

The panels, by key: `1` cluster, `2` nodes, `3` pods, `4` workloads,
`5` events, `6` k3s (on k3s only), `7` top pods (with metrics-server).
`?` lists each with its kubectl command.

### Lists

In pods, deployments, services, namespaces, events and nodes:

| Key | Action                                                                  |
| --- | ----------------------------------------------------------------------- |
| `/` | filter by substring, ignoring case; `enter` keeps it, `esc` drops it    |
| `w` | auto-refresh on or off; the hint shows `watch 5s`, `watch 20s (slow API)` or `paused` |
| `r` | refresh now                                                             |

| In a pod row | Action                                                         |
| ------------ | -------------------------------------------------------------- |
| `enter`      | describe                                                       |
| `l`          | tail logs; a pod with several containers opens a picker, init containers as `init:<name>` |
| `e`          | `kubectl exec -it -- sh` (`cmd.exe` in a Windows pod)          |
| `f`          | port-forward to the pod                                        |

| In a deployment row | Action                                                  |
| ------------------- | ------------------------------------------------------- |
| `enter`             | describe                                                |
| `l`                 | tail logs of every pod its ReplicaSets run              |
| `f`                 | port-forward to a backing pod                           |

| In a service row | Action                                                     |
| ---------------- | ---------------------------------------------------------- |
| `enter`          | describe                                                   |
| `l`              | tail logs of every pod behind it, finished ones skipped    |
| `f`              | port-forward to the service (default `local = remote + 8000` if remote < 1024) |

| Other views             | Action                                                |
| ----------------------- | ----------------------------------------------------- |
| `enter` in `:ns`        | pods of that namespace (`<all>` for every namespace)  |
| `/` in `:ns`            | type a namespace, when RBAC forbids listing them      |
| `enter` in `:events`    | describe the object the event is about                |
| `W` in `:events`        | only warnings, or every type again                    |
| `enter` in `:nodes`     | describe the node                                     |
| `n` in `:top`           | pods or nodes                                         |
| `s` in `:top`           | sort by CPU or memory                                 |
| `enter` in `:ctx`       | switch to that context                                |
| `r` in `:ctx`           | read the kubeconfig again                             |

| In the forwards view (`:pf`) | Action                          |
| ---------------------------- | ------------------------------- |
| `enter`                      | start a stopped forward         |
| `s`                          | stop a running forward          |
| `r`                          | restart                         |
| `d`                          | delete from state               |
| `ctrl+r`                     | refresh                         |

### Logs

| Key                | Action                                                  |
| ------------------ | ------------------------------------------------------- |
| `↑` `↓` `pgup` `pgdn` | scroll; scrolling up pauses auto-follow, the bottom resumes it |
| `f`                | auto-follow on or off                                   |
| `w`                | soft-wrap on or off                                     |
| `←` / `→`          | horizontal scroll, with wrap off                        |
| `t`                | full pod names instead of the colour tag (several pods) |
| `/`                | search, smart-case: case matters only with an upper-case letter |
| `n` / `N`          | next / previous match                                   |
| `c`                | clear the search, then the buffer                       |
| `p`                | previous instance of the container, and back (one pod)  |

The buffer keeps the last 5000 lines (8 MiB at most), and the tail you ask
for is shared out over the pods. Deployment and service logs look for their pods again every 10 s, so
the pods of a rollout join; `kubectl logs -l` does not follow new pods.

### Describe and YAML

| Key                         | Action                                     |
| --------------------------- | ------------------------------------------ |
| `↑` `↓` `pgup` `pgdn` (`k` `j`) | scroll                                 |
| `g` / `G`                   | top / bottom                               |
| `/`                         | search, ignoring case                      |
| `n` / `N`                   | next / previous match                      |
| `r`                         | fetch again, keeping the scroll position   |
| `esc`                       | clear the search, then back                |

`o` in describe opens the object's YAML; in the YAML view it fetches it
again. Describe runs `kubectl describe` with the footer's exact flags and
says in its title who wrote the text (`via kubectl v1.36.0` or
`built-in`).

## Themes & accessibility

k4s picks its colours to match the terminal background. Override it with
`K4S_THEME` or `--theme`:

| `K4S_THEME`      | Colours                                                   |
| ---------------- | --------------------------------------------------------- |
| `auto` (default) | follow the terminal background                            |
| `dark`           | tuned for dark backgrounds                                |
| `light`          | tuned for light backgrounds                               |
| `mono`           | none: bold, underline and reverse video only              |
| `high-contrast`  | like `auto`, with text at 7:1 and borders at 3:1 or better |

`auto` and `high-contrast` ask the terminal for its background when the TUI
starts and hold the first frame up to 150 ms for the answer; until then
they go by `COLORFGBG`, else dark. Inside tmux or screen, on `TERM=dumb`,
under `CI` or when stdout is not a terminal they do not ask. A terminal
that does not answer gets the dark colours: on a light theme set
`K4S_THEME=light` (PowerShell: `$env:K4S_THEME = "light"`).

Setting [`NO_COLOR`](https://no-color.org) gives `mono`; `--theme` wins over
both `K4S_THEME` and `NO_COLOR`. The default colours keep text at 4.5:1 or
better on Solarized Dark and Light, black, white, Campbell, VS Code dark,
Dracula and Gruvbox.

No state is told by colour alone. The status row and the dashboard lead
every state with a glyph, and mono adds bold (`△`), bold and underline
(`▲`) and reverse video (the row under the cursor, also marked `›`):

| Glyph | Meaning                                                      |
| ----- | ------------------------------------------------------------ |
| `●`   | ok                                                           |
| `△`   | waiting or degraded; a warning                               |
| `▲`   | failing; an error                                            |
| `○`   | off or absent                                                |
| `◇`   | worth a look, such as a recent restart                       |
| `◆`   | a neutral fact                                               |
| `·`   | minor, such as an evicted pod                                |
| `—`   | forbidden or unknown                                         |
| `≈`   | worked out by k4s, or a command that does a little more or less than k4s |

## Quickstart (local k3s)

To try k4s against a throwaway cluster, or to hack on it: requires Go 1.26+,
Docker (tested on OrbStack), and `kubectl` on PATH.

```bash
# k3s v1.36 in docker with traefik, servicelb, metrics-server and local-path;
# applies the demo workloads, writes .kube/config, waits until all are Ready
make demo

# run k4s against that cluster
KUBECONFIG=$(pwd)/.kube/config make run
```

`make k3s-down` stops the cluster and keeps its state, `make k3s-clean` wipes
it. `make k3s-up` only waits for a kubeconfig written by the current k3s
start and a live API, so a file left by an earlier cluster is never used.

`K3S_PROFILE` picks the cluster shape, e.g. `make demo K3S_PROFILE=agents`:

| `K3S_PROFILE`    | Cluster                                                       |
| ---------------- | ------------------------------------------------------------- |
| *(unset)*        | one server, every bundled component                           |
| `minimal`        | no traefik, servicelb or metrics-server                       |
| `agents`         | server + 2 agent nodes                                        |
| `minimal,agents` | both                                                          |

Traefik answers on `localhost:18080` / `18443` and the `nginx-lb`
LoadBalancer Service on `localhost:18090` (`K4S_HTTP_PORT`, `K4S_HTTPS_PORT`,
`K4S_LB_PORT` override them).

### Restricted and exec-plugin kubeconfigs

Kubeconfigs for the ServiceAccounts in `deploy/seed/90-restricted-rbac.yaml`,
with 24h tokens (rerun the target to refresh). Neither account can read
outside `k4s-demo`, so `kubectl get pods -A` is Forbidden.

| Target                       | Writes             | Acts as                                                             |
| ---------------------------- | ------------------ | ------------------------------------------------------------------- |
| `make kubeconfig-restricted` | `.kube/restricted` | `k4s-viewer`: built-in `view` + exec / port-forward in `k4s-demo`   |
| `make kubeconfig-narrow`     | `.kube/narrow`     | `k4s-narrow`: pods only (no logs, services, deployments, events)    |
| `make kubeconfig-plugin`     | `.kube/plugin`     | `k4s-viewer` through an exec credential plugin that logs to stderr and takes 3s |
| `make kubeconfig-plugin-tty` | `.kube/plugin-tty` | the same plugin, but it refuses to run without a terminal           |

### Other clusters

- `make kind-up` / `make kind-down`: a vanilla [kind](https://kind.sigs.k8s.io)
  cluster, kubeconfig in `.kube/kind`; seed it with
  `make seed KUBECONFIG_PATH=$(pwd)/.kube/kind`. Skipped when kind is missing.
- If your Docker host cannot run the bundled components, use
  `K3S_PROFILE=minimal`, or [k3d](https://k3d.io):
  `k3d cluster create k4s --image rancher/k3s:v1.36.5-k3s1 && k3d kubeconfig get k4s > .kube/config && make seed`.

## Demo workloads

`make seed` applies the manifests in `deploy/seed/`:

- `k4s-demo` and `k4s-broken` namespaces
- `nginx` Deployment (3 replicas) + Service + ConfigMap
- `web-with-sidecar` — pod with two containers for container-switching tests
- `log-spammer` — two pods, one log line per second, ideal for multi-log streaming
- `crash-loop` — exercises the `CrashLoopBackOff` rendering
- `named-port` — a Service whose targetPorts are container port names, like Traefik's
- `init-crashloop` — a failing init container, stuck in `Init:CrashLoopBackOff`
- `native-sidecar` — a sidecar declared as an init container with `restartPolicy: Always`
- `nginx-lb` — a LoadBalancer Service served by ServiceLB (`<pending>` on `minimal` or kind)
- a Secret and a CronJob (every 5 minutes) for read-only browsing
- `k4s-viewer` and `k4s-narrow` ServiceAccounts for the restricted kubeconfigs

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
make lines       # no non-test .go file over 800 lines
make cover       # per-package test coverage
make ci          # tidy-check + line cap + vet + lint + race tests
                 # + idle-cpu harness lint and tests + vuln
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

How the pieces fit together, and why, is in
[docs/architecture.md](./docs/architecture.md).

```
cmd/k4s/                entry point
internal/
  cli/                  command-line flags
  startup/              kubeconfig lookup, starting namespace, preflight
  cluster/              distribution detection (k3s, rke2, EKS, GKE, ...)
                        and the hints worded for it
  k8s/                  client-go wrapper: lists, describe, YAML, events,
                        nodes, log targets and streams, metrics,
                        port-forward, API error classification
    dash/               dashboard data layer: refresh plan, Table lists,
                        snapshot, severity, verdict, next suggestions
  kubectl/              the kubectl command behind every action: argv,
                        shell quoting, --context/--kubeconfig, runner
  sanitize/             text from the cluster made inert before it is drawn
  forwards/             port-forward state file + Manager (intents persist
                        across launches; SPDY sessions are in-process)
  logsink/              client-go and plugin output kept off the TUI
  version/              build version (ldflags stamp, else module version)
  tui/                  Bubble Tea root: router, header, footer, history,
                        prompts, exec, exit recap
    views/{dashboard,pods,deployments,services,namespaces,events,nodes,
           top,contexts,describe,yaml,logs,forwards}/
    async/              tokens, bounded calls, in-flight actions, poller
    overlay/            command bar, prompts, confirms, help
    status/             the footer's status row
    pager/              scroll and search for describe and YAML
    viz/                dashboard widgets: meters, sparklines, phase bar
    {stale,filter,command,keys,styles,tuitest}/
deploy/                 demo manifests, dev scripts, kubeconfig fixtures
tools/                  lint and vuln tools in their own go.mod, plus the
                        idle CPU harness (make idle-cpu)
docker-compose.yml      k3s for local development
Makefile                build / test / k3s / quality / release targets
.goreleaser.yaml        release archives + checksums
```

## Status

Release notes for every version are on the
[releases page](https://github.com/LywwKkA-aD/k4s/releases);
[CHANGELOG.md](./CHANGELOG.md) has the short version, with what comes in
1.2.0 under Unreleased.

Next, in 1.3: delete, scale and rollout restart behind a confirmation that
shows the exact command, read-only ConfigMap and Secret views, and a
Homebrew tap. Roadmap on <https://k4scli.io/changelog/>.

## License

MIT — see [LICENSE](./LICENSE).
