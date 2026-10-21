package deployments_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestLogsLastWins: l on api, then l on worker before api answers. Only
// worker's picker opens, whichever reply lands first.
func TestLogsLastWins(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...))
	m := mounted(t, c, shop) // rows: api, idle, nosel, worker
	m, apiReply := tuitest.Step(t, m, tuitest.Press("l"))
	m, _ = tuitest.Drive(t, m, tuitest.Keys("down", "down", "down")...)
	m, workerReply := tuitest.Step(t, m, tuitest.Press("l"))

	for _, order := range [][]tea.Msg{slices.Concat(apiReply, workerReply), slices.Concat(workerReply, apiReply)} {
		_, out := tuitest.Drive(t, m, order...)
		want := views.ContainerPromptRequestMsg{
			Namespace: shop, Pods: []string{"worker-h1-a"}, Containers: []string{appContainer, sidecarContainer},
			NextKind: "logs", Target: ref("worker"), Logs: target(t, c, "worker"),
		}
		if got := tuitest.Requests(out); !only(out, want) {
			t.Errorf("want only worker's picker, got:\n%s", tuitest.Describe(got))
		}
	}
}

// TestLogsRepeatOnTheSameRowResolvesOnce: a second l on the row being
// resolved starts nothing.
func TestLogsRepeatOnTheSameRowResolvesOnce(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...)), shop)
	m, first := tuitest.Step(t, m, tuitest.Press("l"))
	if len(first) != 1 {
		t.Fatalf("l should start one resolve:\n%s", tuitest.Describe(first))
	}
	m, second := tuitest.Step(t, m, tuitest.Press("l"))
	if len(second) != 0 {
		t.Fatalf("a repeat l resolved again:\n%s", tuitest.Describe(second))
	}
	m, out := tuitest.Drive(t, m, first...)
	if got := tuitest.All[views.TailPromptRequestMsg](out); len(got) != 1 {
		t.Errorf("want one tail prompt, got:\n%s", tuitest.Describe(out))
	}
	if _, again := tuitest.Step(t, m, tuitest.Press("l")); len(again) != 1 {
		t.Errorf("an answered row cannot be asked again:\n%s", tuitest.Describe(again))
	}
}

// TestLogsSaysItIsResolvingAfter150ms: a slow resolve shows progress, a
// fast one does not.
func TestLogsSaysItIsResolvingAfter150ms(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...))
	m, clk := mountedOn(t, c, shop)
	m, reply := tuitest.Step(t, m, tuitest.Press("l"))
	if early := clk.Advance(149 * time.Millisecond); len(early) != 0 {
		t.Fatalf("status timer fired early: %v", early)
	}
	m, out := tuitest.Drive(t, m, clk.Advance(time.Millisecond)...)
	want := views.StatusMsg{Level: status.Info, Text: "resolving logs for deployment shop/api…"}
	if got := statuses(out); !slices.Equal(got, []views.StatusMsg{want}) {
		t.Fatalf("statuses = %+v, want %+v", got, want)
	}

	m, _ = tuitest.Drive(t, m, reply...)
	m, reply = tuitest.Step(t, m, tuitest.Press("l"))
	m, _ = tuitest.Drive(t, m, reply...)
	if _, late := tuitest.Drive(t, m, clk.Advance(150*time.Millisecond)...); len(late) != 0 {
		t.Errorf("an answered resolve still reported progress:\n%s", tuitest.Describe(late))
	}
}

// TestLogsReplyForARowTheCursorLeft: the prompt still opens, marked as
// not the selected row.
func TestLogsReplyForARowTheCursorLeft(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...))
	m := mounted(t, c, shop)
	m, reply := tuitest.Step(t, m, tuitest.Press("l"))
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"))
	_, out := tuitest.Drive(t, m, reply...)
	want := views.TailPromptRequestMsg{
		Namespace: shop, Pods: []string{"api-h1-a", "api-h1-b"}, Container: appContainer,
		Target: ref("api"), Logs: target(t, c, "api"), Detached: true,
	}
	if !tuitest.Contains(out, want) {
		t.Errorf("want %+v among:\n%s", want, tuitest.Describe(out))
	}
}

// TestLogsWithGuessedOwnersSaysHowToCheck: without list on replicasets
// the pods are matched by name, and the status says what to ask RBAC.
func TestLogsWithGuessedOwnersSaysHowToCheck(t *testing.T) {
	t.Parallel()

	forbidden := tuitest.WithReactor("list", "replicasets", func(k8stesting.Action) (bool, runtime.Object, error) {
		gr := schema.GroupResource{Group: "apps", Resource: "replicasets"}
		return true, nil, apierrors.NewForbidden(gr, "", errors.New("no grant"))
	})
	c := tuitest.NewFakeClient(t, tuitest.WithObjects(shopObjects()...), forbidden)
	guessed := target(t, c, "api")
	if !guessed.OwnersGuessed {
		t.Fatal("fixture: owners were not guessed")
	}
	_, out := tuitest.Drive(t, mounted(t, c, shop), on("api", tuitest.Press("l"))...)
	wantPrompt := views.TailPromptRequestMsg{
		Namespace: shop, Pods: []string{"api-h1-a", "api-h1-b"}, Container: appContainer,
		Target: ref("api"), Logs: guessed,
	}
	wantStatus := views.StatusMsg{Level: status.Info, Text: "owners guessed: kubectl auth can-i list replicasets -n shop"}
	if !tuitest.Contains(out, wantPrompt) || !tuitest.Contains(out, wantStatus) {
		t.Errorf("want the prompt and %q, got:\n%s", wantStatus.Text, tuitest.Describe(out))
	}
}
