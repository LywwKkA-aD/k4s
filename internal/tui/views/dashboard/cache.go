package dashboard

import (
	"sync"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

// The body is drawn again only when something it is drawn from changed:
// a poll's tick, an unchanged answer or a key that moves nothing hands
// back the last frame as it was, so the renderer gets a byte-identical
// View and skips it (§8: an unchanged poll costs no render).

// renderKey is everything a frame is drawn from that can change while
// one dashboard is on screen. The snapshot counts through its section
// hashes; the client, the session and the context are fixed per
// dashboard, and the memo is too.
type renderKey struct {
	hash        dash.Hashes
	scope       dash.Scope
	caps        dash.Caps
	daily       string
	loaded      bool
	focus       focus
	lay         layoutKey
	since       time.Time
	changed     time.Time
	paused      bool
	pausedAt    time.Time
	staleSince  time.Time
	retry       time.Duration
	hint        string
	beat        string
	prog        float64
	glow        glow
	toast       async.Token
	ringLen     int
	ringAt      time.Time
	fresh       int
	histAt      time.Time
	notOKAt     time.Time
	topByMemory bool
	theme       string
	distro      cluster.Distro
}

// layoutKey is what a layout is built from.
type layoutKey struct {
	w, h       int
	sh         shown
	open, zoom dash.Panel
}

// keyOf is f's renderKey for layout l.
func keyOf(f frame, l layout) renderKey {
	k := renderKey{
		hash: f.snap.Hash, scope: f.snap.Scope, caps: f.snap.Caps, daily: f.snap.Daily.Cmd.String(),
		loaded: f.loaded, focus: f.focus,
		lay:   layoutKey{w: l.w, h: l.h, sh: l.sh, open: l.open, zoom: l.zoom},
		since: f.since, changed: f.changed, paused: f.paused, pausedAt: f.pausedAt,
		staleSince: f.staleSince, retry: f.retry, hint: f.hint, beat: f.beat, prog: f.prog, glow: f.glow,
		toast: f.toast.tok, ringLen: len(f.ring), fresh: len(f.fresh),
		histAt: f.hist.sampledAt, notOKAt: f.hist.notOKAt, topByMemory: f.topByMemory, distro: f.distro,
		// Two glyphs in the active theme's styles stand for the theme: a
		// new theme repaints even where nothing else changed.
		theme: f.ds.OK.Render(markGlyphs[markOK]) + f.ds.Title.Render(markGlyphs[markFact]),
	}
	if len(f.ring) > 0 {
		k.ringAt = f.ring[0].at
	}
	return k
}

// renderMemo holds the last frame drawn and its key. View is a value
// method, so the model holds this one pointer; the lock makes that safe
// wherever View runs.
type renderMemo struct {
	mu   sync.Mutex
	key  renderKey
	body string
	ok   bool
}

// get returns the frame drawn for k, if it is the last one.
func (c *renderMemo) get(k renderKey) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ok || c.key != k {
		return "", false
	}
	return c.body, true
}

// put keeps body as the frame drawn for k.
func (c *renderMemo) put(k renderKey, body string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.key, c.body, c.ok = k, body, true
}
