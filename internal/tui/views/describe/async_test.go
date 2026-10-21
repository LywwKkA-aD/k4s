package describe_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
)

// TestReplyForAnotherObjectIsDropped: the describe of web-1 that lands in
// the view opened for web-2 is not shown there.
func TestReplyForAnotherObjectIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	forWeb1 := tuitest.Collect(t, describe.New(c, describe.KindPod, tuitest.NamespaceDemo, tuitest.PodWeb1).Init())
	if len(forWeb1) != 1 {
		t.Fatalf("Init should fetch once:\n%s", tuitest.Describe(forWeb1))
	}
	web2 := describe.New(c, describe.KindPod, tuitest.NamespaceDemo, tuitest.PodWeb2)
	web2, _ = tuitest.Drive(t, web2, tuitest.Resize(100, 40))
	ownReply := tuitest.Collect(t, web2.Init())

	web2, _ = tuitest.Drive(t, web2, forWeb1...)
	if screen := tuitest.Screen(web2); !strings.Contains(screen, "loading describe") {
		t.Fatalf("web-1's describe landed in web-2's view:\n%s", screen)
	}
	web2, _ = tuitest.Drive(t, web2, ownReply...)
	screen := tuitest.Screen(web2)
	if !strings.Contains(screen, tuitest.PodWeb2) || strings.Contains(screen, tuitest.PodWeb1) {
		t.Errorf("web-2's own describe:\n%s", screen)
	}
}

// TestRefreshWhileFetchingIsIgnored: r during a fetch would only race it;
// the answer on its way is already fresh.
func TestRefreshWhileFetchingIsIgnored(t *testing.T) {
	t.Parallel()

	m := describe.New(tuitest.NewFakeClient(t), describe.KindPod, tuitest.NamespaceDemo, tuitest.PodWeb1)
	first := tuitest.Collect(t, m.Init())
	m, again := tuitest.Step(t, m, tuitest.Press("r"))
	if len(again) != 0 {
		t.Errorf("r during the first fetch fetched again:\n%s", tuitest.Describe(again))
	}
	m, _ = tuitest.Drive(t, m, first...)
	if _, out := tuitest.Step(t, m, tuitest.Press("r")); len(out) != 1 {
		t.Errorf("r after the answer should fetch once:\n%s", tuitest.Describe(out))
	}
}
