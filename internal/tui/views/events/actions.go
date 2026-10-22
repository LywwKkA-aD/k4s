package events

import (
	"errors"
	"fmt"
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
	ref, err := objectRef(e)
	if err != nil {
		return statusCmd(status.Warn, "event "+sanitize.Line(e.Name)+" names "+err.Error())
	}
	return func() tea.Msg {
		return views.DescribeRequestMsg(ref)
	}
}

// objectRef is the object e is about as kubectl describe takes it. Anyone
// who may create events writes involvedObject, so every part is checked:
// a name that reads as a flag or a kind that is no identifier never
// reaches kubectl.
func objectRef(e k8s.Event) (views.ResourceRef, error) {
	o := e.Object
	group := ""
	if before, _, found := strings.Cut(o.APIVersion, "/"); found {
		group = before
	}
	switch {
	case !kindPattern.MatchString(o.Kind),
		group != "" && kubectl.CheckSubdomain("group", group) != nil,
		kubectl.CheckSubdomain(o.Kind, o.Name) != nil:
		return views.ResourceRef{}, errNoObject
	}
	kind := strings.ToLower(o.Kind)
	if !plainGroups[group] {
		kind += "." + group
	}
	ns, err := objectNamespace(e, group, kind)
	if err != nil {
		return views.ResourceRef{}, err
	}
	return views.ResourceRef{Kind: kind, Namespace: ns, Name: o.Name}, nil
}

// objectNamespace is the namespace of the object e is about, kind as
// kubectl takes it. An involvedObject may leave it out: a namespaced kind
// then lives in the event's own namespace and a cluster-scoped one in
// none. A kind k4s does not know, such as most custom resources', cannot
// be placed: describing it in the context's namespace would describe
// another object or none, so the error names the kubectl describe that
// works if the kind is cluster-scoped.
func objectNamespace(e k8s.Event, group, kind string) (string, error) {
	o := e.Object
	ns := o.Namespace
	if ns == "" {
		switch k8s.KindScope(group, o.Kind) {
		case k8s.ScopeCluster:
			return "", nil
		case k8s.ScopeUnknown:
			return "", fmt.Errorf("%s/%s without a namespace; if it is cluster-scoped: %s",
				o.Kind, o.Name, kubectl.New("describe", kind, o.Name).String())
		}
		ns = e.Namespace
	}
	if kubectl.CheckLabel("namespace", ns) != nil {
		return "", errNoObject
	}
	return ns, nil
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
