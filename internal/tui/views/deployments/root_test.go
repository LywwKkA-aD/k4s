package deployments_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const rootWidth = 220

// footer is the root's footer below its rule: the hints row and the
// kubectl row.
func footer(m tui.Model) string {
	lines := strings.Split(tuitest.Screen(m), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" && strings.Trim(s, "─") == "" {
			return tuitest.TrimLines(strings.Join(lines[i+1:], "\n"))
		}
	}
	return ""
}

// TestEscFromLogsBringsTheListBack is the M3.9 scenario through the root:
// filter "pay", cursor on the third match, watch paused, l into the logs,
// Esc. The list comes back with the same filter, row and watch state, and
// the footer reads as it did.
func TestEscFromLogsBringsTheListBack(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithObjects(payObjects()...))
	m, _ := tuitest.Mount(t, tui.New(c, nil), rootWidth, height)
	m, _ = tuitest.Drive(t, m, tuitest.Press(":"), tuitest.Type("deploy"), tuitest.Enter())
	m, _ = tuitest.Drive(t, m, on("pay", tuitest.Press("down"), tuitest.Press("down"), tuitest.Press("w"))...)
	before := footer(m)
	tuitest.Golden(t, "footer_filtered_paused", before+"\n")

	m, out := tuitest.Drive(t, m, tuitest.Press("l"), tuitest.Enter())
	logs, ok := tuitest.Find[views.LogsRequestMsg](out)
	if !ok || logs.Target != ref("pay-worker") {
		t.Fatalf("l did not open pay-worker's logs:\n%s", tuitest.Describe(out))
	}
	if strings.Contains(tuitest.Screen(m), "/ pay") {
		t.Fatalf("still on the list:\n%s", tuitest.Screen(m))
	}

	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if after := footer(m); after != before {
		t.Errorf("footer after Esc:\n%s\nwant\n%s", after, before)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "/ pay") || strings.Contains(screen, " web ") {
		t.Errorf("filter not restored:\n%s", screen)
	}
	_, out = tuitest.Drive(t, m, tuitest.Enter())
	want := views.DescribeRequestMsg{Kind: "deployment", Namespace: shop, Name: "pay-worker"}
	if !tuitest.Contains(out, want) {
		t.Errorf("the cursor left pay-worker; enter asked:\n%s", tuitest.Describe(tuitest.Requests(out)))
	}
}
