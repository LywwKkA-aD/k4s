# k4s architecture

How k4s is put together, and why. This is the map; the package docs
(`go doc ./internal/...`) are the territory. Where they disagree, the code
wins and this file is out of date.

k4s is one binary with no server side: `client-go` against a kubeconfig,
and `kubectl` as a subprocess for exec and describe. It polls; it never
keeps a watch or an informer open.

## Packages and their boundaries

```
cmd/k4s ──► cli, styles.Init, logsink, startup ──► tui (tea.Program)

internal/tui ─────────────► views/* ─────┐
  router, chrome, history,    (siblings   │
  prompts, exec, recap         never      ├──► async, status, overlay,
                               import     │    pager, filter, stale,
                               each other)│    viz, styles, keys
                                          ▼
          k8s, k8s/dash, kubectl, forwards, cluster, sanitize   (core)
```

depguard keeps two rules in `make lint`:

- a view under `internal/tui/views/<x>` never imports a sibling view; views
  talk to each other only through messages the root handles;
- the core packages (`k8s`, `forwards`, `logsink`, `kubectl`, `cli`,
  `startup`, `cluster`, `sanitize`) never import `internal/tui` or any
  Charm package, so they test headless and a Charm upgrade cannot reach
  them.

## Startup

`main` runs, in order:

1. `cli.Parse`: flags; `--help` and `--version` exit before anything
   touches the terminal or a kubeconfig.
2. `styles.Init`: the theme from `--theme`, `K4S_THEME` and `NO_COLOR`.
   It decides whether the program will ask the terminal for its
   background.
3. `logsink.Install`: klog, `HandleError` and API warnings go to
   `k4s.log` before the first clientset is built (client-go captures
   stderr for credential plugins at build time).
4. `startup.Open`: the kubeconfig lookup (flag, `$KUBECONFIG`,
   `~/.kube/config`, then `/etc/rancher/k3s/k3s.yaml` on Linux) and the
   starting namespace (`-A`, else `-n`, else the context's, else all). It
   makes no API call.
5. `startup.Preflight` (unless `--no-preflight`): only for contexts with an
   exec or auth-provider plugin, run once on the plain terminal so a login
   prompt can show, bounded at 30 s.
6. `tea.NewProgram` with `tea.WithFPS(20)`. After it returns, `main`
   prints the exit recap, the held plugin output and the log summary.

## The views contract

Every screen implements `views.View` (`internal/tui/views/view.go`):

```go
type View interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (View, tea.Cmd)
    View() string          // the body only; the root draws the frame
    Title() string         // header label and history routing key
    Command() kubectl.Cmd  // footer row 2, what y copies
    Help() []key.Binding   // the view's keys for the hints and ?
    CapturesKeys() bool    // a focused input takes every key
    Close() error          // stop streams and goroutines; idempotent
}
```

`View` mirrors `tea.Model`, but `Update` returns a `View`, so the root
keeps the concrete screen without type assertions, and only the root
returns a `tea.View` (alternate screen, window title, focus reporting).

The root looks for optional interfaces on top:

| Interface              | What the root does with it                                   |
| ---------------------- | ------------------------------------------------------------ |
| `Stateful`, `Restorer` | keeps a `views.State` (committed filter, row key, cursor, paused watch, mode, sort) in the history entry, and hands it to a freshly built view before `Init` on Esc |
| `BackHandler`          | asks the view first on Esc: a list clears a kept filter, describe and YAML a search, the dashboard steps zoom → row → panel |
| `Selection`            | `o` asks it for the object under the cursor and opens its YAML |
| `HelpMap` (the dashboard) | adds a map of the view's parts to the help popup          |

`State` is a comparable value. The row is a key (`ns/name`), never an
index: rows move between polls. A restored view applies its filter at
once, holds the row until its first list lands, and when the row is gone
lands on the old index with "deployment web is gone".

Views never reach into each other. They ask the root with messages:
`DescribeRequestMsg`, `YAMLRequestMsg`, `OpenViewMsg` (the dashboard's
drill-down), `TailPromptRequestMsg`, `ContainerPromptRequestMsg`,
`LogsRequestMsg`, `ExecRequestMsg`, `ForwardRequestMsg`,
`NamespaceSelectedMsg`, `NamespaceFallbackMsg`, `ContextSelectedMsg`.
Messages are values: the sender never touches a slice it sent.

Three messages put text in the status row:

- `StatusMsg{Level, Text}`: the answer to something the user did. A key
  that cannot do its job says why ("deployment web has 0 replicas"); it
  never returns nil.
- `StatusAboutMsg`: a `StatusMsg` about a command. While it shows, the
  footer's command row shows that command and `y` copies it ("describe
  node needs kubectl"). The root does the same for the `kubectl debug`
  hint after a missing shell.
- `ToastMsg`: news nobody asked for, such as a change a dashboard poll
  found. It never replaces an error still waiting for a key; failing news
  goes at `status.Alert`.

## Keys

```
key press
   │
   ├─ ctrl+c ─────────────────────────────────────────► quit
   ├─ an overlay is open ─► y over one without a text input copies;
   │                        anything else goes to the overlay
   ├─ view.CapturesKeys() ─► every key to the view (filter, search, prompt)
   ├─ q : ? esc y o ───────► the root (home/quit, command bar, help,
   │                         back, copy, YAML)
   └─ anything else ───────► the view
```

The root owns `q`, `:`, `?`, `Esc`, `y`, `o` and `ctrl+c`, and no view
binds them. `CapturesKeys` is load-bearing: without it, `q` typed into a
filter would go home. The root also normalises keys first
(`keys.Normalize`): with the kitty keyboard protocol, shift+Enter and
ctrl+Enter arrive as distinct keys and must still act as Enter.

History holds at most 50 entries. The lists, the dashboard, contexts and
forwards are entries by name; describe and YAML by name and object. Logs
push their parent but are never entries: a stream is not reopened behind
the user's back. The same view and namespace on top replace the top entry.
`:ctx` drops the history with every saved State. The dashboard's
`Memory` lives in a field of its own, not in history, so it survives `q`
(which clears history) and is dropped only by `:ctx`.

Overlays (`internal/tui/overlay`) are the command bar, which takes the
footer's first row, and the popups: tail prompt, container picker, port
prompt, confirms (restore, login) and help. At most one is open; it gets
every key and paste, closes by returning nil, and answers with a message
(`TailChosenMsg`, `ForwardChosenMsg`, ...). Confirms ignore keys for their
first 400 ms. An overlay that previews a command (the tail prompt, the
container picker) puts it in the footer instead of the view's.

## Chrome: header, footer, status row

```
 k4s · k3s v1.36.5 · ctx default (10.0.0.5) · ns ALL · pods      header
────────────────────────────────────────────────────────────
 body: the view's View(), or a popup over it
────────────────────────────────────────────────────────────
 row 1: command bar | status message | key hints
 row 2: the kubectl command, decorated for this session
```

The header and both footer rows are cut to the width, so the body height
never depends on what they say. The header gives up, in order, the
release, the distribution, the middle of the context name, then the host.
After every view switch the root sends itself a `relayoutMsg` that
re-delivers a body-sized `WindowSizeMsg`; a synthetic `tea.WindowSizeMsg`
would shrink the stored height by the chrome on every switch.

The status row (`internal/tui/status`) holds one message:

| Level   | Glyph | Stays                                             |
| ------- | ----- | ------------------------------------------------- |
| `Info`  | none  | 5 s                                               |
| `OK`    | `●`   | 5 s                                               |
| `Warn`  | `△`   | 5 s                                               |
| `Error` | `▲`   | until a key is pressed, and never less than 5 s   |
| `Alert` | `▲`   | 5 s; drawn as an error, but no key is waiting on it |

Text is folded onto one line, stripped of escape sequences and control
characters, and `$HOME` is shortened to `~`. The glyph means no level is
told by colour alone.

## Async: tokens, bounded calls, pollers

`Update` never blocks. Every cluster call runs in a `tea.Cmd` through
`internal/tui/async`:

- **`Token{owner, seq}`.** `async.New` gives each view instance its own
  owner; `Next` starts a new epoch. A reply is taken only under the token
  it was started with, so a describe meant for pod A never lands in the
  view of pod B, and a tick armed by one view is dropped by the next.
- **`Run[T](tok, timeout, fn)`.** One call with a deadline, answered as
  `Result[T]`. The reply comes shortly after the deadline even when `fn`
  ignores its context (a credential plugin waiting on a browser), so a hung
  call can never leave a poller busy. A panic in `fn` is raised again on
  the Cmd goroutine, where Bubble Tea restores the terminal.
- **`InFlight`.** Row actions in flight, by name. `Begin` drops a second
  request (a second `e` must not open a second shell); `Supersede` lets
  the newest win (`l` on another row) and refuses a repeat for the same
  row.
- **`Poller`.** The refresh cadence as a value: tick, busy flag,
  generation, timeout and backoff.

```
            Begin (first load, r)           Due(tick)
   idle ──────────────────────► busy ◄──────────────── armed
    ▲                            │                       ▲
    └──── disabled/inactive ─────┤ Done(err)             │
                                 └── enabled & active ───┘
                                     arm next tick (5 s × backoff)
```

The next tick is armed when a poll is answered, never when the last tick
fired, so polls cannot overlap and a slow answer pushes the next one back.
A poller arms only while it is enabled (watch on, a client), active (on
screen) and idle. `Classify` turns the error into an outcome: a timeout or
failed dial is `Slow` and doubles the interval 5, 10, 20, 40, 60 s; a 429
is `Throttled` and doubles the whole poller, a `Retry-After` (capped at
5 minutes) floors the next tick; any answer, even an error, resets the slow
streak. `Hint` gives the `w` label (`watch 5s`, `watch 20s (slow API)`,
`paused`): that is where watch state shows, since no footer command
carries `--watch`. Polls read the watch cache (`resourceVersion=0`); `r`
asks for a consistent read. Views take a clock option, so tests drive
ticks by hand.

A list whose poll fails keeps its last good rows under a one-line stale
banner (`internal/tui/stale`). Every error is worded by `k8s.Classify`:
forbidden (verb, resource, scope and the matching `kubectl auth can-i`),
unauthorized, login needed, not found, timeout, unreachable, throttled,
unavailable. When `-A` is forbidden and the context names a namespace, the
list falls back to it and sends `NamespaceFallbackMsg`, so the header and
the history entry follow.

## The kubectl command model

`internal/kubectl` is the one place a kubectl command is built. A `Cmd` is
argv, never a string:

```go
type Cmd struct {
    Args     []string // the words after "kubectl"
    Mutating bool     // changes cluster state (for a future confirm)
    Approx   bool     // k4s does a little more or less than this
    // context and kubeconfig are private: only Decorate sets them
}
```

- `String()` is one POSIX shell line that, pasted, runs exactly `Argv()`;
  words are quoted only where a shell would change them. `Display()` adds
  `≈` for an approximate command; the footer shows `Display`, `y` copies
  `String`.
- Views never add `--context` or `--kubeconfig`. The root runs every
  command through `Decorate(cmd, session)`: `--context` when the context is
  not the kubeconfig's current-context (after `:ctx`) or came from
  `--context`; `--kubeconfig`, or `KUBECONFIG=a:b` for several files, when
  k4s loaded other files than a bare kubectl in the same shell would.
- Builders (`PodLogs`, `SelectorLogs`, `Exec`) refuse a namespace or
  container that is not a DNS-1123 label and an object name that is not a
  DNS-1123 subdomain. Names come from the cluster; one such as `--all`
  would otherwise read as a flag. A refused command is the zero `Cmd`, an
  empty footer row.
- `CheckFlags` is the backstop: a word before `--` that sets a session
  flag (`--server`, `--kubeconfig`, `--context`, `--token`, `--as`, `-s`
  in any shorthand cluster, and the like) fails the command. `y`, the exit
  recap and the Runner all check it; `y` also refuses a command that holds
  control characters, since quoting keeps a name one argument but does not
  make it inert once pasted.
- `Runner` runs argv with `exec.Command`, never a shell. It looks up
  `kubectl` on every call and falls back to `k3s kubectl`, pins
  `KUBECONFIG` to the files k4s loaded, keeps stdin at the null device,
  caps stdout at 8 MiB and stderr at 64 KiB, and times out at 10 s. Exec
  uses `Runner.Command` with `tea.ExecProcess`, so the terminal goes to
  kubectl and comes back. Describe runs `kubectl describe` through it,
  decorated exactly as the footer shows it, and falls back to the built-in
  describer (pods, deployments, services) when kubectl is missing, fails or
  times out.

Every command a session leads to is recorded once (`recap`) and printed
after the TUI exits, at most 15, least recently used first; one that sets
a session flag is left out.

## Sanitize

`internal/sanitize` turns text from the cluster into inert text. A
workload controls what it writes to its log, and anyone who may create an
event controls its message, so without it a pod could set the operator's
clipboard over OSC 52, retitle or clear the terminal, or draw over k4s.

- `Line` removes escape sequences with their payload (CSI, OSC, DCS, SOS,
  PM, APC and short ESC sequences), every other C0 and C1 control but
  tab, CR and LF included, and invisible format characters (bidi controls,
  zero-width characters, tags, the soft hyphen). Invalid UTF-8 becomes
  U+FFFD; a line over 64 KiB is cut and noted `…[+N bytes]`.
- `Text` does the same for several lines, keeping LF; no sequence runs
  past the end of its line.
- Clean ASCII is returned without a copy, which keeps log streaming cheap.

It runs on log lines as they arrive, on events, describe and YAML output,
on every dashboard string before layout, and on the header, the window
title and the footer's command row. The package has no Charm imports, so
`k8s` and `kubectl` use it too (kubectl's stderr, error hints).

## The dashboard

The data layer, `internal/k8s/dash`, has no Charm imports. One refresh is
three values in and out:

```
in   := snap.PlanInput(elapsed, watch, force, focus)
plan := dash.MakePlan(in)                 // pure: due set + next interval
snap  = fetch(ctx, plan, snap)            // requests, then a new Snapshot
```

- **Plan.** `MakePlan` reads no clock and starts nothing. Each endpoint has
  a tier: fast 5 s (pods, deployments), medium 15 s (node metrics,
  statefulsets, daemonsets, pod metrics), slow 30 s (nodes, warning events,
  PVCs, failed jobs, cronjobs, LoadBalancers, readyz, namespaces, k3s Helm
  charts; the service count every minute and k3s etcd snapshots every
  5 minutes), and once per context (`/version`, SelfSubjectReview, metrics
  discovery). The fast tier stretches to 15 s
  while the terminal is blurred or after 10 idle minutes, and to 15 or
  30 s on large clusters (rows, bytes or wall time); nodes past 20 go to
  60 s; more than 2000 warnings are read as a capped page. 403 and 404 are
  parked for 5 minutes and asked again on `r` or a scope change. With watch
  off, or the dashboard off screen, `Arm` is false: no tick at all.
- **Ledger.** One `Usage` per endpoint (fetched, when, state, rows, bytes,
  wall time, stale since, last error) carries what the plan needs from one
  refresh to the next.
- **Fetch.** At most eight requests at once. Lists are server-side Tables
  (`as=Table`, `includeObject=None`) read by column name, so a missing
  column drops its metric instead of guessing; typed lists go over
  protobuf through a dashboard-only copy of the config. A Table without
  objects has no namespace or containers, so the few flagged rows get a
  typed lookup by name, at most ten per refresh; healthy rows cost nothing.
  Polls read `resourceVersion=0`; the `limit=1` service count and `r` read
  consistently, since `resourceVersion=0` ignores `limit`.
- **Snapshot.** Every panel as data, each panel and row with its exact
  read-only kubectl command (the verb allow-list: get, describe, logs,
  top, events, version, cluster-info, api-resources, auth can-i and
  whoami) and an `Approx` flag where k4s summarises. Denied, absent,
  not-ready, stale and unanswered sources are panel states, never errors
  or zeros. Each section carries an FNV-64 hash.
- **Severity.** `●` ok, `◇` info, `·` minor, `△` waiting, `▲` failing,
  one glyph per value. Crash loops, image pulls and config errors fail;
  pods merely starting stay out of the verdict for 2 minutes; a hot meter
  and its chip are both `△`; a warning event is `△` at most and counts only
  when recent, on an object that exists and no other panel flags.
- **Verdict, next, command of the day.** The verdict is the chips by
  severity; all clear needs no chip, fresh sources and no scope fallback.
  `next` is at most five suggestions, one per object, by severity then
  recency; a logs suggestion always names its container. With nothing to
  fix, a command of the day comes from a curated list of read-only
  commands, filtered by what the cluster offers (metrics, k3s, etcd).

The view, `internal/tui/views/dashboard`, owns one `async.Poller` driven
by the plan (`WithInterval` after every snapshot and on focus changes).
`Memory` (snapshot, ledger, history rings, focus, the change ring)
passes from one dashboard to the next. Layouts are recomputed only on a
size message: wide, medium, compact accordion, summary lines and a
minimal verdict, with panel numbers that never change. The cursor is keyed
by object, so a poll that re-sorts rows keeps it on the same pod.

Motion runs on one clock: a single `animFrameMsg` every 50 ms, armed only
while something moves, with a frame budget per event (meters spring up on
the first snapshot in at most 8 frames, a focus sweep takes 6). An event
during motion joins the running clock. There is no motion on the ASCII and
NoTTY profiles, while the terminal is blurred, or while the median of the
last 16 renders is above 4 ms; mono keeps the springs but no gradients.

The idle guarantees, which the dashboard's idle-guard tests pin:

- watch off: no tick is armed and no request is sent; `Update` runs only
  for keys and terminal events;
- watch on and nothing changing: only the poll's ticks and replies reach
  `Update`, no animation frame is armed, and `View` hands back the cached
  frame byte for byte (a frame is drawn again only when a section hash, the
  focus, the layout, the history, the motion, a toast or the theme
  changed), which the renderer skips;
- the floor: Bubble Tea's renderer still wakes 20 times a second
  (`tea.WithFPS(20)`) for the whole session. `make idle-cpu` measures the
  real cost on a pty.

## Logs streaming

The logs view (`internal/tui/views/logs`) runs a `session`: one goroutine
per pod reads its stream, a pump hands lines to the view in batches of at
most 1000 per drain, and the view applies a batch with one render. Every
batch carries the session's token, so a batch from a closed session or
another logs view is never taken.

```
pod readers ──► [merger] ──► events chan ──► view: append, cap, render
  (one per pod;   (several      (batched)      backlog ≤ 5000 lines, 8 MiB
  bufio, 64 KiB   pods only)
  per line kept)
```

- Lines are read with a bounded `bufio.Reader`: a line keeps its first
  64 KiB and the rest is counted, so a 2 MiB line neither ends the stream
  nor holds 2 MiB. Every line is made inert on the way in.
- Streams always ask for timestamps. The stamp is cut before the line is
  sanitised and kept as the pod's resume point; the footer never shows
  `--timestamps`.
- Several pods are merged in two phases without comparing the laptop's
  clock with a node's: each pod's history is held until it sent its tail,
  went quiet for 300 ms or ended (5 s at most), then goes out sorted by
  line time; later lines wait up to 250 ms in a heap ordered by line time.
- A deployment's pods are those its ReplicaSets control (controller UID
  chain), listed by the deployment's bare selector so a new ReplicaSet's
  pods are seen; the footer narrows the selector with
  `pod-template-hash in (…)`. A service's pods are its selector's minus
  finished ones. `--max-log-requests` counts every pod the shown selector
  matches, finished ones included, up to 50, the most k4s streams.
- A following view resolves its pods again every 10 s, only while open:
  one list of pods from the watch cache, ReplicaSets again only for a pod
  of one it has not seen (a rollout). New pods stream from their creation.
  That is at most six lists a minute, and none after `Close`.
- An ended stream is judged by the next check: deleted, completed, failed,
  restarted (reattach), exited (wait), or dropped (reconnect after 1, 2,
  4, 8, then 30 s, asking from the resume point and dropping the lines
  already shown). `--previous` streams are read once.
- Each reader, the merge loop and the previous-instance reader recover a
  panic: the stack goes to `k4s.log` and the stream ends with an internal
  error, instead of k4s dying with the terminal in raw mode.

## Port-forward manager and state file

`internal/forwards` keeps two things apart: the persisted intent (which
forwards the user declared) and the in-process sessions (SPDY tunnels that
live only as long as k4s).

```
 Stopped ── Start ──► Starting ── ready ──► Running
                          │                    │
                          └─────► Error ◄──────┘
                       error, exit or lost connection

 Stop takes any of them back to Stopped; restart is a stop, then a start.
```

- The `Manager` drives a `Tunneler` (the part of `*k8s.Client` that
  resolves a pod and opens a tunnel), which tests replace with fake
  sessions. Every read returns a copy; `Changes()` is a channel closed and
  remade on every change, which the forwards view waits on.
- Each `Start` numbers its attempt; a late report from an attempt the user
  has since stopped or restarted is ignored.
- The dial goes the way kubectl's does (SPDY over WebSocket, falling back
  to SPDY) under a 15 s startup bound that a stop also cancels. The
  forwarder's output goes to a bounded ring; failures read off each
  connection become the DETAIL column's `last error`.
- `:ctx` calls `BeginStopAll`, which closes every session at once and
  returns a `Stopping` handle waited on in a Cmd under one 2 s deadline. The
  next context's Manager is built after it and refuses `Register` and
  `Start` until the old sessions are down, so two contexts never race for a
  local port.
- The state file is `$XDG_STATE_HOME/k4s/portforwards.json` (the variable
  counts only when absolute; default `~/.local/state`). A forward records
  its context, API server and a SHA-256 fingerprint of the CA: stock k3s
  calls every context `default` at `127.0.0.1:6443`, and only the CA tells
  two clusters apart. `Register` and `Remove` save first and change memory
  only after the save succeeded. `Save` writes a temp file, syncs it,
  renames it and syncs the directory (and every directory it created); the
  directory loses group and other permissions. Every stored forward is
  validated on load; a file that fails is renamed to
  `portforwards.json.corrupt-<time>` and k4s starts without saved forwards,
  saying so once.
- Panics in the forwarder, the stream wrappers, the supervisor and
  `Start` end that forward with an internal error, never k4s.

## Testing

`make test` runs everything with the race detector; no test touches a real
cluster unless asked to.

- **`internal/tui/tuitest`.** `Drive` feeds messages through the root or a
  view the way `tea.Program` does: every Cmd runs (batches concurrently,
  sequences in order) and what it produces is fed back, each Cmd bounded at
  250 ms. `Init`, `Step` and `Collect` cover the other shapes. Key and
  paste builders, `Screen` and `Footer` (ANSI-stripped), `HeaderView`,
  `Clipboard`, goroutine baselines, `Isolate` (a `TestMain` that points
  `KUBECONFIG` and `XDG_STATE_HOME` away from the developer's files) and
  a fake client on `fake.NewSimpleClientset` with an in-memory metrics API
  and a demo seed.
- **Goldens.** `tuitest.Golden` compares with `testdata/<name>.golden`;
  `go test <package> -run <Test> -update` rewrites them (name the package:
  only test binaries that define the flag know it). They are ANSI-free.
  The dashboard's matrix renders five synthetic fixtures (busy k3s, all
  clear, namespace-scoped EKS, k3s without metrics, vanilla kind) at
  several sizes, checks that every line is as wide as the body, and that
  the colour render stripped of escapes equals the mono render.
- **Clocks.** `asynctest.Clock` is a manual `async.Clock`: a tick
  registers its timer and returns at once, and the test fires or advances
  time. `asynctest.Instant` fires at once, for motion frames. Views and the
  root take clocks as options, so tests step time by hand.
- **`internal/k8s/k8stest`.** Reactors with the API server's own wording
  (`RestrictToNamespace`, `Forbid`, `Unauthorized`, `SlowerThanDeadline`)
  and an httptest-backed client for paths a fake cannot serve (metrics,
  403, 404, 503, a server that never answers). The dashboard's
  request-budget tests count requests and decoded bytes per minute against
  a counting test API server.
- **Against a cluster.** A few read-only tests run only when
  `K4S_CLUSTER_KUBECONFIG` points at one (the logs ones also need
  `-tags cluster`). `make idle-cpu` runs `bin/k4s` on a pty against
  `make demo` and reports CPU and RSS over 40 s.
