package events

import (
	"errors"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// errNoObject is an involvedObject kubectl could not be asked about.
var errNoObject = errors.New("no object kubectl can describe")

// kindPattern is what a kind can be: an identifier, as every API kind is.
var kindPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// plainGroups are the API groups whose kinds kubectl and the built-in
// describer take by the bare lower-case kind; a kind of any other group
// is named kind.group, which never resolves to another group's kind.
var plainGroups = map[string]bool{"": true, "apps": true}

// row is the event under the cursor. Without one, cmd is the status that
// says why, so a row key never fails silently.
func (m Model) row() (e k8s.Event, cmd tea.Cmd, ok bool) {
	switch {
	case m.client == nil:
		return e, statusCmd(status.Warn, "no kubeconfig"), false
	case !m.loaded:
		return e, statusCmd(status.Info, "events are still loading"), false
	case m.health.Failed():
		return e, statusCmd(status.Warn, "events unavailable, press r"), false
	}
	if e, ok = m.current(); !ok {
		return e, statusCmd(status.Warn, "no event selected"), false
	}
	return e, nil, true
}

// describe is Enter: describe the object the event is about.
func (m Model) describe() tea.Cmd {
	e, cmd, ok := m.row()
	if !ok {
		return cmd
	}
	ref, err := objectRef(e.Object)
	if err != nil {
		return statusCmd(status.Warn, "event "+sanitize.Line(e.Name)+" names "+err.Error())
	}
	return func() tea.Msg {
		return views.DescribeRequestMsg(ref)
	}
}

// objectRef is the object o names as kubectl describe takes it. Anyone
// who may create events writes involvedObject, so every part is checked:
// a name that reads as a flag or a kind that is no identifier never
// reaches kubectl.
func objectRef(o k8s.EventObject) (views.ResourceRef, error) {
	group := ""
	if before, _, found := strings.Cut(o.APIVersion, "/"); found {
		group = before
	}
	switch {
	case !kindPattern.MatchString(o.Kind),
		group != "" && kubectl.CheckSubdomain("group", group) != nil,
		kubectl.CheckSubdomain(o.Kind, o.Name) != nil,
		o.Namespace != "" && kubectl.CheckLabel("namespace", o.Namespace) != nil:
		return views.ResourceRef{}, errNoObject
	}
	kind := strings.ToLower(o.Kind)
	if !plainGroups[group] {
		kind += "." + group
	}
	return views.ResourceRef{Kind: kind, Namespace: o.Namespace, Name: o.Name}, nil
}

// Selected implements views.Selection: the event under the cursor, so o
// opens its YAML with the whole message. There is none before the first
// list lands or when the filter leaves no row.
func (m Model) Selected() (views.ResourceRef, bool) {
	e, ok := m.current()
	if !m.loaded || !ok {
		return views.ResourceRef{}, false
	}
	return views.ResourceRef{Kind: "event", Namespace: e.Namespace, Name: e.Name}, true
}
