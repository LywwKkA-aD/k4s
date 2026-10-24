# Changelog

All notable changes to k4s are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). Full release notes
are on the [releases page](https://github.com/LywwKkA-aD/k4s/releases).

## [Unreleased]

### Behaviour changes

- The home screen is a new dashboard (see Added); the four counters are
  gone.
- k4s starts in the namespace the kubeconfig context sets, as kubectl does,
  and `:ctx` lands in the new context's namespace. A context without a
  namespace still starts in all namespaces; `-A` or `:ns -A` shows all
  namespaces.
- Footer commands no longer end in `--watch`: k4s refreshes by polling,
  which `kubectl get --watch` does not do. The `w` hint shows the refresh
  state instead: `watch 5s`, `watch 20s (slow API)` or `paused`.
- `l` on a deployment streams the pods its ReplicaSets run, not every pod
  its labels match, so CronJob pods that share the label stay out. `l` on a
  service skips finished pods. When `kubectl logs -l` would stream more
  pods, the footer marks the command `≈` and the status row says how many
  were skipped and why.
- Describe runs `kubectl describe` (or `k3s kubectl describe`) exactly as
  the footer shows it. The built-in describer answers only when kubectl is
  missing, fails or takes more than 10 s; the footer then marks the command
  `≈` and the status row says why.
- The question about saved port-forwards answers yes on Enter only: `y`
  now copies the command.

### Added

- A dashboard as the home screen (`q`, `:dashboard`, `:home`):
  - The banner answers whether the cluster is healthy: a verdict
    (`▲ attention` when something fails, `△ attention` when the worst is
    waiting, followed by what is wrong; or `● all clear since hh:mm` with
    what k4s saw), up to five `▸ next` suggestions, each an exact read-only
    kubectl command with the question it answers, and a command of the day
    when nothing needs a fix.
  - Seven numbered panels: 1 cluster (API readiness, version and
    distribution, user, age, namespaces, storage, network, server), 2 nodes
    (condition, roles, CPU and memory meters, pods per node), 3 pods (phase
    bar, problem pods, recent restarts), 4 workloads (deployments,
    statefulsets, daemonsets, jobs, cronjobs), 5 events (recent warnings),
    6 k3s (bundled components, ServiceLB, Helm charts, etcd snapshots; on
    k3s only), 7 top pods (with metrics-server).
  - The footer shows the kubectl command of the focused panel or row, and
    `y` copies it. Enter opens the matching k4s view (`:pods`, `:deploy`,
    `:events` with only the warnings, `:nodes`, `:ns`, `:svc`, `:top`) or
    describes the row's object; `l` opens a pod row's logs, the previous
    instance's for a crash loop suggestion; `o` opens the row's YAML; `z`
    zooms a panel. `tab`/`shift+tab`, `0`-`7`, `←`/`→` and `↑`/`↓` move the
    focus, and `?` lists each panel with its key and command.
  - What k4s works out itself is marked `≈`: CPU and memory history, the
    count of pods not ok, and "seen by k4s", the changes between two polls.
    It lives in memory for the session and is still there when you come
    back from another view. A change between polls raises one toast.
  - Wide, medium and compact layouts fit every terminal size; panel
    numbers never change with the size.
  - A denied source reads `—` with the `kubectl auth can-i` that asks the
    same, never `0`. A user limited to one namespace sees that namespace
    and a note in the banner. A missing metrics-server says why, with the
    install or re-enable hint for the distribution. A source that stops
    answering keeps its last data, dimmed and marked `stale`.
  - Requests come in tiers of 5 s, 15 s, 30 s and once per context and
    read the API server's watch cache; pod metrics are asked only while
    panel 7 shows. Polls slow down on large clusters, while the terminal
    is in the background, after 10 minutes without a key, on timeouts and
    on 429s. With watch off nothing is polled.
- `o` opens the YAML of the object under the cursor, or of the object
  describe shows: `kubectl get <kind> <name> -o yaml` for pods,
  deployments, statefulsets, daemonsets, jobs, cronjobs, services,
  namespaces, nodes and events. It is built in-process and leaves out
  `managedFields`, as kubectl does.
- Describe and YAML scroll with `↑`/`↓`, `pgup`/`pgdn`, `g` and `G`, and
  search with `/`, `n` and `N`; Esc clears a search before it leaves.
- `:events` (`:ev`): `kubectl events`, newest first. `W` keeps only the
  warnings (`--types=Warning`), Enter describes the object an event is
  about, `/` filters.
- `:nodes` (`:no`): `kubectl get nodes -o wide`, with k3s roles and
  cordoned nodes shown as `SchedulingDisabled`. Enter describes the node.
- `:pods`, `:deploy`, `:svc`, `:top` and `:events` take a namespace as
  kubectl spells it: `:pods demo`, `:deploy -n demo`, `:svc -A`.
- Flags `--kubeconfig`, `--context`, `-n`/`--namespace`,
  `-A`/`--all-namespaces`, `--theme`, `--no-preflight` (also
  `K4S_NO_PREFLIGHT`) and `--no-recap` (also `K4S_NO_RECAP`). A
  `--kubeconfig` or `--context` that cannot be loaded stops k4s before the
  TUI starts.
- On exit k4s prints the kubectl commands the session led to (the views
  opened, describe, logs, exec and port-forward), each once, the 15 most
  recent, so they stay in the shell's scrollback. `--no-recap` turns it
  off.
- `--theme high-contrast` (or `K4S_THEME=high-contrast`): text at 7:1 and
  borders at 3:1 or better, following the terminal background like `auto`.
- On a k3s server without `$KUBECONFIG` or `~/.kube/config`, k4s reads
  `/etc/rancher/k3s/k3s.yaml`. When that file is not readable, k4s prints
  how to make a readable copy.
- The header names the distribution and release, and the API host when the
  context name alone does not say which cluster it is:
  `k4s · k3s v1.36.5 · ctx default (10.0.0.5) · ns ALL · pods`. The
  terminal title reads `k4s · <context>/<namespace>`.
- `y` copies the footer's kubectl command to the clipboard through the
  terminal (OSC 52); inside tmux this needs `set -g set-clipboard on`.
- `:ns <name>` and `:ns -A` switch the namespace of the current list.
- A status row in the footer. Actions that cannot run say why, and a failed
  credential preflight is shown there when the TUI starts. Messages lead
  with a glyph for their level (`●` ok, `△` warning, `▲` error), so no
  level depends on colour.
- Esc brings a list back as it was left: its filter, the row under the
  cursor, watch, and top's mode and sort. Esc on a kept filter clears it
  first.
- Logs:
  - Deployment and service logs look for their pods again every 10 s while
    the view is open, so the pods of a rollout join the stream
    (`+ pod …-k7x2p joined (new ReplicaSet 7d9f…)`). `kubectl logs -l`
    does not follow new pods, and the help says so.
  - Every stream that ends says why: the pod was deleted, completed or
    failed (`Evicted`, `Error (exit 1)`), its container restarted (the
    stream reattaches), exited (it waits for the restart), or the stream
    dropped (it reconnects after 1, 2, 4, 8, then 30 s without repeating a
    line). The bare `stream closed` line is gone.
  - `p` shows the previous instance of a single pod's container
    (`kubectl logs --previous`); `p` again goes back.
  - The container picker lists init containers and native sidecars as
    `init:<name>`.
  - At most 50 pods stream at once, and the footer asks kubectl for no
    more.
- `s` in `:top` sorts by memory and back; the footer command always carries
  `--sort-by`.
- A list whose refresh fails keeps its last rows under a stale banner, and
  they stay usable.
- When RBAC forbids listing namespaces, `:ns` opens a prompt to type one,
  with the namespaces used this session to pick from.
- After `:ctx` to a context whose credential plugin needs a terminal login,
  a question offers it; Enter runs `kubectl version` for that context on
  the plain terminal. A preflight that failed for want of a login at start
  leads to the same question.
- Exec runs `cmd.exe` in Windows pods. A container without a shell ends in
  a status that offers the `kubectl debug` command bringing one; `y` copies
  it, and k4s never runs it.
- The forwards view's DETAIL column shows a forward's last connection
  failure (`last error: connection refused · 12s ago`).

### Changed

- The footer is always two rows: key hints or a status message, then the
  kubectl command.
- Footer commands can be pasted as they are: they carry `--context` when
  k4s was started with `--context` or talks to another context than the
  kubeconfig's current-context, and `--kubeconfig` when
  k4s reads a kubeconfig that plain `kubectl` would not (`--kubeconfig`,
  the k3s fallback). `≈` now marks only commands that differ from what k4s
  does.
- The multi-pod logs footer is a runnable command instead of a list of pods
  in braces: `kubectl logs -f -l '<selector>' --prefix --tail=<n>
  --max-log-requests=<m>`, narrowed for a deployment to its ReplicaSets
  with `pod-template-hash in (…)`.
- API errors are worded by what went wrong: forbidden (with the verb,
  resource and scope, and the `kubectl auth can-i` that asks the same),
  unauthorized, login needed, not found, timeout, unreachable, throttled or
  unavailable. A refused `127.0.0.1:6443` off a k3s server points at a
  copied `k3s.yaml`.
- When RBAC refuses a list across all namespaces and the context names a
  namespace, the pods, deployments, services, top and events views list
  that namespace and say so (`cluster-wide list forbidden; showing demo ·
  kubectl auth can-i list pods -A`), and the header moves there too.
  Without a context namespace the error points at `:ns <name>`.
- Lists read their polls from the API server's watch cache, and `r` asks
  for a consistent read. With watch off a view polls nothing and arms no
  timer. A slow API backs the refresh off to 10, 20, 40 and 60 s.
- The pod list's READY, STATUS and RESTARTS columns count the way
  `kubectl get pods` does, including init containers, native sidecars,
  `Terminating` and `Evicted`. RESTARTS reads `3 (5m ago)`, ages print as
  kubectl prints them (`90m`, `3d12h`), and the pod table fits 80
  columns.
- The tail prompt names what the logs are for (`deployment/web · ns demo`)
  and its line cap (5000 lines shared out over the pods), and the footer
  shows the logs command it will open.
- `e` offers only the containers a shell runs in: app containers and native
  sidecars.
- A port-forward prompt with a bad or busy port stays open and shows the
  error.
- Saved port-forwards are bound to their cluster: context, API server and
  CA. Stock k3s names every context `default` at `127.0.0.1:6443`, so a
  forward saved on one k3s cluster no longer comes back on another. Forwards
  saved by earlier releases read `unknown cluster, confirm` and are bound
  when started.
- The question about saved port-forwards names the clusters it offers
  (`default · 127.0.0.1:6443`) and ignores keys pressed in its first
  400 ms.
- Port-forwards connect as kubectl does (SPDY over WebSocket, else SPDY).
  A start gives up after 10 s; the dial itself is bounded at 15 s.
- Help closes on `?`, Esc or `q`; any other key closes it and does what it
  does without help open. It always fits the screen, dropping detail on a
  small terminal.
- Exec runs exactly the footer's command, without a shell, and uses
  `k3s kubectl` when `kubectl` is not on `PATH`.
- `:ctx` lists and loads contexts from the kubeconfig k4s started with.
- The credential plugin login before the TUI starts gives up after 30 s;
  `--no-preflight` skips it.
- `auto` and `high-contrast` ask the terminal for its background once the
  TUI starts, and hold the first frame up to 150 ms for the answer.
  Windows terminals are asked too; tmux, screen and `TERM=dumb` are not.
- Built on Bubble Tea, Lip Gloss and Bubbles v2, with these visible effects:
  - Truecolor terminals get the exact palette colours (v1 was one step off in some channels), and inside tmux `COLORTERM=truecolor` is honoured.
  - Pasting into the command bar, filters and the logs search works through bracketed paste.
  - With the kitty keyboard protocol (Alacritty, kitty, WezTerm, ghostty), shift+Enter and ctrl+Enter still act as Enter.
  - Under `CI=1` or `TERM=screen` the palette is shown instead of no styling, and East Asian ambiguous-width locales truncate and box consistently.
  - Without a terminal (ssh without `-t`, cron) the error reads `no terminal: open /dev/tty: …`.
- `k8s.io/*` v0.37.1.

### Fixed

- A panic in a log stream or a port-forward no longer ends k4s with the
  terminal in raw mode: that stream or forward ends with
  `internal error (see ~/.local/state/k4s/k4s.log)`, and the stack goes to
  the log.
- A log line longer than the reader's buffer no longer ends its stream:
  each line keeps its first 64 KiB. The log buffer holds at most 5000
  lines and 8 MiB, so a flood of long lines no longer freezes the screen.
- Multi-pod history is merged by line time without comparing a node's
  clock with the laptop's, so a node minutes off no longer scrambles it.
- Logs search is smart-case, counts every match, and highlights only the
  log text; `n` and `N` reach matches in wrapped lines, and a paused view
  keeps its place while old lines scroll out.
- A header whose cluster did not answer `/version` at start asks again
  after 15 s, 30 s and then every minute, instead of staying unnamed.
- `:ctx` no longer waits for port-forwards that hang: the status row says
  `stopping N forwards…` while they close (2 s at most), and a new forward
  waits until they have.
- A forward that fails to start, or a saved one that cannot be revived,
  says why in the status row.
- A corrupt port-forward state file is moved aside to
  `portforwards.json.corrupt-<time>` and reported at startup; k4s starts
  with no saved forwards instead of disabling them.
- A dial that times out reads as a slow API, not an unreachable one.
- The built-in describer shows an object's newest 500 events and says how
  many there were. Describe of a node has no `-n`.

### Security

- Text from the cluster is made inert before k4s draws it: log lines,
  events, describe and YAML output, object names, the header, the
  footer's command row and the terminal title. Escape sequences (clipboard
  writes over OSC 52, title changes, screen clears) and control, bidi and
  zero-width characters are dropped, invalid UTF-8 becomes U+FFFD, and a
  line is cut at 64 KiB.
- `y`, the exit recap and every kubectl k4s runs refuse a command in which
  a word from the cluster would set a session flag such as `--server`,
  `--kubeconfig`, `--token` or `-s`; `y` also refuses one that holds
  control characters. Names that are not valid Kubernetes names never
  reach a kubectl command.
- Errors shown in the TUI lose URL queries and credentials.
- The port-forward state file is synced before and after it is replaced,
  its directory is kept to its owner, and every saved forward is checked
  on load.
- The dashboard reads a single node annotation,
  `etcd.k3s.cattle.io/local-snapshots-timestamp`, and shows only the user
  name from `kubectl auth whoami`.

## [1.1.5] - 2026-10-20

### Fixed

- Nothing writes over the TUI any more. client-go (klog) errors and Kubernetes
  API warnings go to `$XDG_STATE_HOME/k4s/k4s.log` (default
  `~/.local/state/k4s/k4s.log`); on exit k4s prints how many messages were
  logged and where. Credential plugin output (EKS, GKE, OIDC) is held in
  memory while the TUI runs and printed after exit, never logged; contexts
  that use such a plugin are contacted once before the TUI starts, so login
  prompts appear on the plain terminal.
- Readable colours on dark and light terminal themes. The background is
  detected at startup; `K4S_THEME=auto|dark|light|mono` overrides it and
  `NO_COLOR` selects `mono` (bold, underline and reverse video, no colour)
  ([#3](https://github.com/LywwKkA-aD/k4s/issues/3)).
- Service port-forward targets the Service port you typed instead of the
  Service's first port, and resolves a named `targetPort` against the pod's
  container ports.
- Exec runs `kubectl` against the same kubeconfig as the TUI, and says so when
  `kubectl` is not on `PATH`.
- The dashboard shows the real kubeconfig error instead of a generic hint.

### Added

- `--version` and `--help` flags.
- Prebuilt binaries for Linux (amd64, arm64, armv7), macOS (amd64, arm64) and
  Windows (amd64), with a SHA-256 `checksums.txt`, attached to each release.

### Changed

- client-go rate limits raised to QPS 50 / burst 100.

## [1.1.4] - 2026-10-20

### Security

- Go toolchain 1.26.10 and `golang.org/x/net` v0.61.0, with refreshed
  `x/sys`, `x/term`, `x/text`, `x/oauth2` and `x/time`. govulncheck found 9
  reachable vulnerabilities in 1.1.3 and finds none now.

### Added

- GitHub Actions quality gate: `make ci` on every push, pull request and
  weekly, plus cross-builds for linux amd64/arm64/armv7, darwin amd64/arm64
  and windows amd64.
- Dependabot: weekly grouped updates for Go modules and GitHub Actions.

### Changed

- Lint and vulnerability tools live in their own `tools/go.mod`, so linter
  upgrades never move app dependencies.
- Tool targets run on the toolchain pinned by `go.mod`, so a newer local Go no
  longer breaks `make lint`.

## [1.1.3] - 2026-08-25

### Fixed

- Multi-pod logs: a pod without the requested container streams its own first
  container instead of failing. This hit services whose selector also matches
  pods of other workloads.

### Changed

- Pod colours in logs come from a hue-separated palette, assigned by position,
  instead of an FNV hash of the pod name. Neighbouring pods never share a hue.

## [1.1.2] - 2026-08-25

### Fixed

- Multi-pod log history is merged by timestamp instead of arriving pod by pod.
- Service port-forwards tunnel to the Service's `targetPort`, not its port.
- Switching contexts stops live forwards and frees their local ports; the
  forwards list and the restore prompt only show the active context.
- A stuck API server fails a port-forward after 15s instead of leaking a
  goroutine.
- Exec passes `--context`, so the shell opens in the cluster selected in k4s.
- The `★` marker no longer hides the first letter of the active context.
- Auto-refreshing views no longer stack refresh loops; they skip a fetch while
  one is in flight and drop replies for a previous namespace.
- `Esc` from describe or logs returns to the previous list.
- Deployment logs and exec support `matchExpressions` selectors.
- `describe` truncation is UTF-8 safe.

### Changed

- Dashboard counts use four `Limit: 1` queries instead of full lists, and a
  denied resource no longer blanks the whole dashboard.
- Go 1.26.7, `k8s.io/*` v0.36.4.

## [1.1.1] - 2026-05-11

### Added

- `l` on a service tails the logs of every pod behind it, like `l` on a
  deployment.

## [1.1.0] - 2026-05-11

### Added

- Port-forward: `f` on a service, pod or deployment row. The local port
  defaults to the remote port, plus 8000 below 1024.
- Forwards view (`:pf`, `:forwards`) to start, stop, restart and delete
  forwards.
- Forwards are saved to `$XDG_STATE_HOME/k4s/portforwards.json` and the next
  launch offers to revive them.
- Logs: soft-wrap (`w`) and horizontal scroll (`←` / `→`).

### Changed

- Logs render in batches, so `--tail=5000` reaches the bottom at once.

## [1.0.0] - 2026-05-08

### Added

- First release: pods, namespaces, deployments and services lists with
  describe; multi-pod log streaming with search, auto-follow and two-step
  clear; exec via `kubectl exec`; `top` for pods and nodes; `/` filter;
  `:ctx` context switcher; `w` auto-refresh; `?` help popup; the `kubectl`
  equivalent of every action in the footer.

[Unreleased]: https://github.com/LywwKkA-aD/k4s/compare/v1.1.5...HEAD
[1.1.5]: https://github.com/LywwKkA-aD/k4s/compare/v1.1.4...v1.1.5
[1.1.4]: https://github.com/LywwKkA-aD/k4s/compare/v1.1.3...v1.1.4
[1.1.3]: https://github.com/LywwKkA-aD/k4s/compare/v1.1.2...v1.1.3
[1.1.2]: https://github.com/LywwKkA-aD/k4s/compare/v1.1.1...v1.1.2
[1.1.1]: https://github.com/LywwKkA-aD/k4s/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/LywwKkA-aD/k4s/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/LywwKkA-aD/k4s/releases/tag/v1.0.0
