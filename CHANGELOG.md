# Changelog

All notable changes to k4s are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). Full release notes
are on the [releases page](https://github.com/LywwKkA-aD/k4s/releases).

## [Unreleased]

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
