# Changelog

All notable changes to k4s are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). Full release notes
are on the [releases page](https://github.com/LywwKkA-aD/k4s/releases).

## [Unreleased]

### Behaviour changes

- k4s starts in the namespace the kubeconfig context sets, as kubectl does,
  and `:ctx` lands in the new context's namespace. A context without a
  namespace still starts in all namespaces; `-A` or `:ns -A` shows all
  namespaces.
- The question about saved port-forwards answers yes on Enter only: `y`
  now copies the command.

### Added

- Flags `--kubeconfig`, `--context`, `-n`/`--namespace`,
  `-A`/`--all-namespaces`, `--theme` and `--no-preflight` (also
  `K4S_NO_PREFLIGHT`). A `--kubeconfig` or `--context` that cannot be
  loaded stops k4s before the TUI starts.
- On a k3s server without `$KUBECONFIG` or `~/.kube/config`, k4s reads
  `/etc/rancher/k3s/k3s.yaml`. When that file is not readable, k4s prints
  how to make a readable copy.
- `y` copies the footer's kubectl command to the clipboard through the
  terminal (OSC 52); inside tmux this needs `set -g set-clipboard on`.
- `:ns <name>` and `:ns -A` switch the namespace of the current list.
- A status row in the footer. Actions that cannot run say why, and a failed
  credential preflight is shown there when the TUI starts.

### Changed

- The footer is always two rows: key hints or a status message, then the
  kubectl command.
- Footer commands can be pasted as they are: they carry `--context` after
  `:ctx` or when k4s was started with `--context`, and `--kubeconfig` when
  k4s reads a kubeconfig that plain `kubectl` would not (`--kubeconfig`,
  the k3s fallback). `≈` now marks only commands that differ from what k4s
  does.
- Footer commands no longer end in `--watch`: k4s refreshes by polling,
  which `kubectl get --watch` does not do.
- The multi-pod logs footer is a runnable command instead of a list of pods
  in braces.
- The pod list's READY, STATUS and RESTARTS columns count the way
  `kubectl get pods` does, including init containers, native sidecars,
  `Terminating` and `Evicted`.
- The tail prompt names what the logs are for (`deployment/web · ns demo`),
  and the footer shows the logs command it will open.
- A port-forward prompt with a bad or busy port stays open and shows the
  error.
- Help closes on `?`, Esc or `q`; any other key closes it and does what it
  does without help open.
- The question about saved port-forwards ignores keys pressed in its first
  400 ms.
- Exec runs exactly the footer's command, without a shell, and uses
  `k3s kubectl` when `kubectl` is not on `PATH`.
- `:ctx` lists and loads contexts from the kubeconfig k4s started with.
- The credential plugin login before the TUI starts gives up after 30 s;
  `--no-preflight` skips it.
- Built on Bubble Tea, Lip Gloss and Bubbles v2, with these visible effects:
  - Truecolor terminals get the exact palette colours (v1 was one step off in some channels), and inside tmux `COLORTERM=truecolor` is honoured.
  - Pasting into the command bar, filters and the logs search works through bracketed paste.
  - With the kitty keyboard protocol (Alacritty, kitty, WezTerm, ghostty), shift+Enter and ctrl+Enter still act as Enter.
  - Under `CI=1` or `TERM=screen` the palette is shown instead of no styling, and East Asian ambiguous-width locales truncate and box consistently.
  - Without a terminal (ssh without `-t`, cron) the error reads `no terminal: open /dev/tty: …`.

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
