package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const (
	headerBrand = "k4s"
	headerSep   = " · "
	// minContextCells is as far as the header shortens a context name
	// before it gives up the host.
	minContextCells = 12
)

// identity is what the header and the window title say about the cluster
// the session talks to. It is worked out when the client changes (start,
// :ctx) and never while rendering, since Ambiguous reads the kubeconfig
// files.
type identity struct {
	// context is the kubeconfig context, "" without a client.
	context string
	// host is the API server's host when the context name alone does not
	// say which cluster this is ("default", or a name two kubeconfig files
	// define); "" otherwise.
	host string
	// version fences the reply to the header's /version request: one sent
	// for a client k4s has since switched away from is dropped.
	version async.Token
}

func newIdentity(c *k8s.Client) identity {
	if c == nil {
		return identity{}
	}
	id := identity{context: sanitize.Line(c.Context), version: async.New()}
	if c.RestConfig != nil && cluster.Ambiguous(c.Context, c.KubeconfigPaths) {
		id.host = cluster.Host(c.RestConfig.Host)
	}
	return id
}

// headerParts are the header's words before they are fitted to the width:
// `k4s · <distro> <release> · ctx <context> (<host>) · ns <ns> · <view>`.
// Empty parts are left out.
type headerParts struct {
	distro  string
	release string
	context string
	host    string
	ns      string
	view    string
}

func (m Model) headerParts() headerParts {
	p := headerParts{
		context: m.ident.context,
		host:    m.ident.host,
		ns:      headerNamespace(m.namespace),
		view:    sanitize.Line(m.current.Title()),
	}
	if m.client == nil {
		return p
	}
	if v := m.startup.ServerVersion; v != nil {
		p.release = cluster.Release(v.GitVersion)
	}
	// "k8s" says nothing until there is a release to go with it.
	if d := m.startup.Distro; d != cluster.Other || p.release != "" {
		p.distro = d.Name()
	}
	return p
}

// headerNamespace is the namespace as the header and the window title show
// it. It comes from the kubeconfig or the command line, so it is made
// inert like the context name.
func headerNamespace(ns string) string {
	if ns == "" {
		return "ALL"
	}
	return sanitize.Line(ns)
}

// layoutHeader fits p into width cells (see fitHeader); a width of 0 or
// less leaves the header whole, as before the first WindowSizeMsg.
func layoutHeader(p headerParts, width int) string {
	if width <= 0 {
		return p.render()
	}
	return ansi.Truncate(fitHeader(p, width).render(), width, "…")
}

// fitHeader shortens p until it fits width, in this order: the release
// goes, then the distribution, then the context name loses its middle
// (down to minContextCells), then the host goes and the context name gets
// the room back. Whatever is still too wide is cut at the end by
// layoutHeader, so the view title goes before the namespace.
func fitHeader(p headerParts, width int) headerParts {
	if p.width() <= width {
		return p
	}
	p.release = ""
	if p.width() <= width {
		return p
	}
	p.distro = ""
	if p.width() <= width {
		return p
	}
	if q := p.shortenContext(width); q.width() <= width {
		return q
	}
	p.host = ""
	return p.shortenContext(width)
}

// shortenContext cuts the middle of the context name by what p is too
// wide for width, keeping at least minContextCells.
func (p headerParts) shortenContext(width int) headerParts {
	if over := p.width() - width; over > 0 {
		p.context = middleTruncate(p.context, max(minContextCells, ansi.StringWidth(p.context)-over))
	}
	return p
}

func (p headerParts) width() int {
	return ansi.StringWidth(strings.Join(p.words(), headerSep))
}

// words are the header's segments as plain text, the brand first.
func (p headerParts) words() []string {
	out := []string{headerBrand}
	if p.distro != "" {
		out = append(out, strings.TrimSpace(p.distro+" "+p.release))
	}
	if p.context != "" {
		ctx := "ctx " + p.context
		if p.host != "" {
			ctx += " (" + p.host + ")"
		}
		out = append(out, ctx)
	}
	out = append(out, "ns "+p.ns)
	if p.view != "" {
		out = append(out, p.view)
	}
	return out
}

func (p headerParts) render() string {
	words := p.words()
	styled := make([]string, 0, len(words))
	styled = append(styled, styles.Title.Render(words[0]))
	for _, w := range words[1:] {
		styled = append(styled, styles.Hint.Render(w))
	}
	return strings.Join(styled, styles.Hint.Render(headerSep))
}

// middleTruncate cuts s to n cells by replacing its middle with "…", so
// both the start and the end stay readable: an EKS ARN ends with the
// cluster's name.
func middleTruncate(s string, n int) string {
	w := ansi.StringWidth(s)
	if w <= n {
		return s
	}
	if n <= 1 {
		return ansi.Truncate(s, n, "")
	}
	head := (n - 1) / 2
	tail := n - 1 - head
	return ansi.Truncate(s, head, "") + "…" + ansi.TruncateLeft(s, w-tail, "")
}

// windowTitle is the terminal's title: "k4s · <context>/<namespace>".
func (m Model) windowTitle() string {
	if m.client == nil {
		return headerBrand
	}
	return headerBrand + headerSep + m.ident.context + "/" + headerNamespace(m.namespace)
}
