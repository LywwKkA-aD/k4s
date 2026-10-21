package views

import tea "charm.land/bubbletea/v2"

// State is how a list view looked when the user left it, so that Esc
// brings it back the same way. It is a plain comparable value: the zero
// State is a fresh view, and the root never restores it.
type State struct {
	// Filter is the committed "/" query, "" for none. A query still being
	// typed is not kept.
	Filter string
	// Selected names the row under the cursor, as RowKey gives it; "" for
	// none. It is a key, not an index: rows move between fetches.
	Selected string
	// Cursor is the cursor's index when the state was taken: where the
	// cursor lands when Selected is gone.
	Cursor int
	// Paused is set when the user turned auto-refresh off with w; the zero
	// State watches, as a fresh view does.
	Paused bool
	// Mode and Sort are settings of one view, such as top's "nodes" mode
	// and its sort column; "" is that view's default.
	Mode string
	Sort string
}

// WithoutSelection returns s without its row: the state :ns <name> carries
// into the same view in another namespace, where the row is not.
func (s State) WithoutSelection() State {
	s.Selected, s.Cursor = "", 0
	return s
}

// RowKey names a row for State.Selected: "namespace/name", or just "name"
// for a cluster-scoped row (namespaces, nodes, contexts).
func RowKey(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}

// Stateful is implemented by views worth bringing back as they were. The
// root takes State when it leaves the view for another one it can return
// from (describe, logs, another list) and keeps it in that history entry;
// :ctx drops every saved State with the history.
type Stateful interface {
	State() State
}

// Restorer is implemented by views that can take a State back. On Esc the
// root builds a fresh view with its usual constructor, calls Restore on it
// before Init, and runs both the returned Cmd and the new view's Init. A
// Restorer
//
//   - applies Filter at once, so its first frame is already filtered;
//   - keeps Paused, Mode and Sort;
//   - holds Selected until its first list arrives, then puts the cursor on
//     that row (the table scrolls it into view);
//   - when that row is gone, lands on the row at Cursor (clamped to the
//     list) and emits a StatusMsg at status.Info such as
//     "deployment web is gone";
//   - fetches as on any refresh tick (ResourceVersion "0").
type Restorer interface {
	Restore(State) (View, tea.Cmd)
}

// BackHandler is implemented by views that use Esc themselves before it
// means "back": a list clears its committed filter. The root asks Back
// first; used reports that the view consumed Esc, and the root then keeps
// the returned View and runs the Cmd instead of leaving. When used is
// false the root ignores the other results and goes back in history.
// While the view CapturesKeys, Esc goes to the view directly (an open
// filter closes) and Back is not asked.
type BackHandler interface {
	Back() (next View, cmd tea.Cmd, used bool)
}
