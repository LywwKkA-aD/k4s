package dashboard_test

import (
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	logsPrevious = "kubectl logs payments-worker-6c9f7d8b5-xk2lp -n shop -c worker --previous"
	dailyCmd     = "kubectl get pods -A '--field-selector=status.phase!=Running,status.phase!=Succeeded'"
)

// bannerLine is row i of the body, stripped.
func bannerLine(t *testing.T, screen string, i int) string {
	t.Helper()
	lines := strings.Split(screen, "\n")
	if i >= len(lines) {
		t.Fatalf("no line %d:\n%s", i, screen)
	}
	return lines[i]
}

// TestNextCycles: ↑↓ on the banner step through the next suggestions.
// The second row shows the one under the cursor and its place in the
// list, and the footer its exact command; up from the first goes back to
// the verdict.
func TestNextCycles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []string
		cmd  string
		line string
		sel  bool
	}{
		{name: "the verdict", cmd: verdictCmd, line: "▸ next 1/5  " + logsPrevious},
		{name: "the first suggestion", keys: []string{"down"}, cmd: logsPrevious, line: "▸ next 1/5  " + logsPrevious, sel: true},
		{name: "the second", keys: []string{"down", "j"}, cmd: "kubectl describe pod api-7f6d9c4b8-2mzqt -n staging", line: "▸ next 2/5  kubectl describe pod api-7f6d9c4b8-2mzqt", sel: true},
		{name: "past the last stays on it", keys: []string{"down", "down", "down", "down", "down", "down"}, cmd: "kubectl describe svc traefik -n kube-system", line: "▸ next 5/5", sel: true},
		{name: "up from the first", keys: []string{"down", "down", "k", "up"}, cmd: verdictCmd, line: "▸ next 1/5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, _ := press(t, mount(t, newFeed(busyK3s), wideW, wideH), tt.keys...)
			if got := command(m); got != tt.cmd {
				t.Fatalf("command = %q, want %q", got, tt.cmd)
			}
			line := bannerLine(t, tuitest.Screen(m), 1)
			if !strings.Contains(line, tt.line) || strings.HasPrefix(line, "›") != tt.sel {
				t.Fatalf("next row = %q, want %q, cursor %v", line, tt.line, tt.sel)
			}
		})
	}
}

// TestEnterOnASuggestion: Enter does what the suggestion's action says;
// l opens logs only where the suggestion reads them. --previous is not in
// k4s's logs view yet, so it opens the current logs and says so.
func TestEnterOnASuggestion(t *testing.T) {
	t.Parallel()

	tail := func(ns, name, container string) views.TailPromptRequestMsg {
		return views.TailPromptRequestMsg{
			Namespace: ns, Pods: []string{name}, Container: container,
			Target: views.ResourceRef{Kind: "pod", Namespace: ns, Name: name},
		}
	}
	tests := []struct {
		name   string
		downs  int
		key    string
		want   any
		status string
	}{
		{name: "logs --previous", downs: 1, key: "enter", want: tail("shop", "payments-worker-6c9f7d8b5-xk2lp", "worker"), status: "--previous"},
		{name: "l on logs --previous", downs: 1, key: "l", want: tail("shop", "payments-worker-6c9f7d8b5-xk2lp", "worker"), status: "--previous"},
		{name: "describe a pod", downs: 2, key: "enter", want: views.DescribeRequestMsg{Kind: "pod", Namespace: "staging", Name: "api-7f6d9c4b8-2mzqt"}},
		{name: "l on describe", downs: 2, key: "l", status: "no logs"},
		{name: "logs of a probe", downs: 3, key: "enter", want: tail("monitoring", "grafana-5d8f7c6b9-wq2lx", "grafana")},
		{name: "a runner command", downs: 4, key: "enter", status: "no k4s view · y copies the command"},
		{name: "describe a service", downs: 5, key: "enter", want: views.DescribeRequestMsg{Kind: "service", Namespace: "kube-system", Name: "traefik"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			keys := append(strings.Split(strings.Repeat("down ", tt.downs), " ")[:tt.downs], tt.key)
			_, out := press(t, mount(t, newFeed(busyK3s), wideW, wideH), keys...)
			if tt.want != nil && !tuitest.Contains(out, tt.want) {
				t.Fatalf("want %#v in:\n%s", tt.want, tuitest.Describe(out))
			}
			msg, ok := tuitest.Find[views.StatusMsg](out)
			if tt.status == "" && ok {
				t.Fatalf("unexpected status %q", msg.Text)
			}
			if tt.status != "" && (!ok || !strings.Contains(msg.Text, tt.status)) {
				t.Fatalf("status %q, want one with %q:\n%s", msg.Text, tt.status, tuitest.Describe(out))
			}
		})
	}
}

// TestLogsOnRows: l on a pod row opens the tail prompt for that pod; on a
// row that is not a pod it says there are no logs.
func TestLogsOnRows(t *testing.T) {
	t.Parallel()

	_, out := press(t, mount(t, newFeed(busyK3s), wideW, wideH), "3", "down", "l")
	want := views.TailPromptRequestMsg{
		Namespace: "shop", Pods: []string{"payments-worker-6c9f7d8b5-xk2lp"},
		Target: views.ResourceRef{Kind: "pod", Namespace: "shop", Name: "payments-worker-6c9f7d8b5-xk2lp"},
	}
	if !tuitest.Contains(out, want) {
		t.Fatalf("l on a pod row:\n%s", tuitest.Describe(out))
	}
	for _, keys := range [][]string{{"1", "down", "l"}, {"l"}} {
		_, out := press(t, mount(t, newFeed(busyK3s), wideW, wideH), keys...)
		if msg, ok := tuitest.Find[views.StatusMsg](out); !ok || !strings.Contains(msg.Text, "no logs") {
			t.Fatalf("l after %v:\n%s", keys, tuitest.Describe(out))
		}
	}
}

// TestHintsOnSuggestions: the footer offers enter and l only where they
// do something on the suggestion under the cursor.
func TestHintsOnSuggestions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		downs    int
		enter, l string
	}{
		{downs: 1, enter: "logs", l: "logs"},
		{downs: 2, enter: "describe"},
		{downs: 4},
	}
	for _, tt := range tests {
		m := mount(t, newFeed(busyK3s), wideW, wideH)
		for range tt.downs {
			m, _ = press(t, m, "down")
		}
		if got, gotL := hint(m, "enter"), hint(m, "l"); got != tt.enter || gotL != tt.l {
			t.Errorf("suggestion %d: enter %q, l %q; want %q, %q", tt.downs, got, gotL, tt.enter, tt.l)
		}
	}
	m, _ := press(t, mount(t, newFeed(busyK3s), wideW, wideH), "3", "down")
	if got := hint(m, "l"); got != "logs" {
		t.Errorf("l hint on a pod row = %q, want logs", got)
	}
}

// TestAllClearBanner: with nothing to fix the verdict says since when and
// what k4s saw, and the second row is the command of the day: a cursor
// stop whose command y copies.
func TestAllClearBanner(t *testing.T) {
	t.Parallel()

	m := mount(t, newFeed(allClear), wideW, wideH)
	screen := tuitest.Screen(m)
	requireAll(t, screen, []string{
		"● all clear since 14:02  3/3 nodes Ready · 185 pods · 65/65 workloads · 2 warnings in 1h",
		"▸ command of the day  " + dailyCmd + "   which pods are neither running nor done?",
		"● nothing needs attention",
	}, []string{"▲ attention", "△ attention"})
	m, out := press(t, m, "down", "enter")
	if got := command(m); got != dailyCmd {
		t.Fatalf("command on the command of the day = %q", got)
	}
	if msg, ok := tuitest.Find[views.StatusMsg](out); !ok || !strings.Contains(msg.Text, "y copies") {
		t.Fatalf("enter on the command of the day:\n%s", tuitest.Describe(out))
	}
}

// TestBannerRightNote: the verdict row ends with when the snapshot last
// changed and the watch state; the narrower layouts keep the time alone,
// and with watch off it says since when it is paused.
func TestBannerRightNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		w, h      int
		keys      []string
		want, not string
	}{
		{name: "wide", w: wideW, h: wideH, want: "◆ changed 14:02:00 · watch 5s ⣽ "},
		{name: "medium", w: 120, h: 34, want: "◆ 14:02:00 · watch 5s ⣽ ", not: "changed"},
		{name: "compact", w: 80, h: 18, want: "◆ 14:02:00 ⣽ ", not: "watch"},
		{name: "paused", w: wideW, h: wideH, keys: []string{"w"}, want: "◆ changed 14:02:00 · ‖ paused 14:02 ⣽ ", not: "watch 5s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, _ := press(t, mount(t, newFeed(busyK3s), tt.w, tt.h), tt.keys...)
			line := bannerLine(t, tuitest.Screen(m), 0)
			if !strings.HasSuffix(line, tt.want) || (tt.not != "" && strings.Contains(line, tt.not)) {
				t.Fatalf("verdict row = %q, want it to end %q without %q", line, tt.want, tt.not)
			}
		})
	}
}

// TestChangedFollowsTheHash: "changed" moves only when a snapshot shows
// something new.
func TestChangedFollowsTheHash(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mount(t, f, wideW, wideH)
	m = advance(t, m, f, 10*time.Second)
	if line := bannerLine(t, tuitest.Screen(m), 0); !strings.Contains(line, "changed 14:02:00") {
		t.Fatalf("unchanged polls moved the time: %q", line)
	}
	f.set(func() dash.Snapshot {
		s := busyK3s()
		s.Pods.Total = 186
		return s
	})
	m = advance(t, m, f, 5*time.Second)
	if line := bannerLine(t, tuitest.Screen(m), 0); !strings.Contains(line, "changed 14:02:15") {
		t.Fatalf("a changed poll did not move the time: %q", line)
	}
}

// TestWordmark: the wide banner opens with the two-row k4s wordmark; the
// cursor takes its first column. Narrower banners have no room for it.
func TestWordmark(t *testing.T) {
	t.Parallel()

	screen := tuitest.Screen(mount(t, newFeed(busyK3s), wideW, wideH))
	if l0, l1 := bannerLine(t, screen, 0), bannerLine(t, screen, 1); !strings.HasPrefix(l0, "›█ ▄ █ █ ▄▀▀   ▲ attention") ||
		!strings.HasPrefix(l1, " █▀▄ ▀▀█ ▄▄▀   ▸ next") {
		t.Fatalf("wide banner:\n%s\n%s", l0, l1)
	}
	m, _ := press(t, mount(t, newFeed(busyK3s), wideW, wideH), "3")
	if l0 := bannerLine(t, tuitest.Screen(m), 0); !strings.HasPrefix(l0, " █ ▄ █ █ ▄▀▀   ") {
		t.Fatalf("banner without focus: %q", l0)
	}
	medium := tuitest.Screen(mount(t, newFeed(busyK3s), 120, 34))
	if strings.Contains(medium, "█▀▄ ▀▀█") {
		t.Fatalf("the medium banner has a wordmark:\n%s", medium)
	}
}
