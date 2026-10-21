// Package views defines the contract every k4s screen implements and the
// cross-view messages screens use to talk to the root model.
//
// # The contract
//
// Every screen is a View. On top of it the root looks for optional
// interfaces:
//
//   - Stateful and Restorer: Esc brings a list back as it was left (the
//     committed filter, the row under the cursor, watch and view
//     settings). The root keeps a State per history entry, hands it
//     to a freshly built view with Restore, and drops them all on :ctx.
//   - BackHandler: Esc is the view's first, so a committed filter clears
//     before Esc leaves the view.
//   - Selection: the object the view shows or the row under its cursor.
//     The root asks it on o and opens that object's YAML; a view without
//     it, or without a row, gets a status instead.
//
// Views never import each other; they ask the root for what lies outside
// them with the messages in this package (DescribeRequestMsg,
// YAMLRequestMsg, TailPromptRequestMsg, LogsRequestMsg, ExecRequestMsg,
// ...).
//
// # Commands
//
// Command returns the kubectl command behind what the view shows, built
// with kubectl.New or a builder (LogsCommand and ExecCommand here,
// kubectl.PodLogs, kubectl.SelectorLogs). It never carries --context,
// --kubeconfig or the approximate mark: the root adds the flags a pasted
// copy needs for this session (kubectl.Decorate) and the footer shows the
// mark when Cmd.Approx is set, which a view sets only where it does more
// or less than the command would. k4s polls, so no command has --watch.
//
// # Rules for views
//
//   - Never fail silently. A key that cannot do its job (no row, a lookup
//     error, 0 replicas, no selector, nothing left to stream) answers with
//     a Cmd that emits StatusMsg, never with nil: "deployment web has 0
//     replicas", "service api has no selector".
//   - Never block in Update. Cluster calls go through internal/tui/async
//     (see its package doc): a Token per view instance fences late
//     replies, InFlight guards row actions such as l and e, and a Poller
//     owns the refresh cadence. A view with a Poller labels its w binding
//     with Poller.Hint() ("watch 5s", "watch 20s (slow API)", "paused"):
//     that is where watch state shows now that the footer has no --watch.
//   - The root owns q, :, ?, Esc, y, o and ctrl+c unless the view
//     CapturesKeys (then every key goes to the view). A view must not bind
//     them. A new global key is added to this list in the same change that
//     binds it in the root.
//   - Time is the view's own. A view that needs a clock for tests takes it
//     as an option on its New (for example WithClock(func() time.Time));
//     the root never passes clocks to views.
//   - Messages are values. A request carries its slices as built and the
//     sender does not touch them afterwards.
package views
