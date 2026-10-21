package tui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// webTarget is the web deployment resolved to n pods of ReplicaSet h1.
func webTarget(n int) (k8s.LogTarget, []string) {
	t := k8s.LogTarget{
		Kind: k8s.LogKindDeployment, Namespace: tuitest.NamespaceDemo, Name: tuitest.DeploymentWeb,
		Selector: "app=web", Hashes: []string{"h1"},
	}
	names := make([]string, 0, n)
	for i := range n {
		name := fmt.Sprintf("web-h1-%02d", i)
		names = append(names, name)
		t.Pods = append(t.Pods, k8s.PodRef{Name: name, Phase: "Running", Hash: "h1"})
	}
	return t, names
}

func webRef() views.ResourceRef {
	return views.ResourceRef{Kind: "deployment", Namespace: tuitest.NamespaceDemo, Name: tuitest.DeploymentWeb}
}

// commandRow is the footer's kubectl row.
func commandRow(m interface{ View() tea.View }) string {
	ls := strings.Split(tuitest.Screen(m), "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		if isRule(ls[i]) && i+2 < len(ls) {
			return strings.TrimSpace(ls[i+2])
		}
	}
	return ""
}

// TestTailPromptFooterPreviewsTheStream: while the prompt is open the
// footer shows the command Enter will open, following the typed count.
func TestTailPromptFooterPreviewsTheStream(t *testing.T) {
	t.Parallel()

	target, pods := webTarget(2)
	req := views.TailPromptRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: pods, Target: webRef(), Logs: target}
	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), req)
	want := "kubectl logs -f -l 'app=web,pod-template-hash in (h1)' --prefix --tail=100 --max-log-requests=2 -n demo"
	if got := commandRow(m); got != want {
		t.Fatalf("command row = %q\nwant          %q", got, want)
	}
	m, _ = tuitest.Drive(t, m, clearInput(), tuitest.Type("20"))
	if got := commandRow(m); !strings.Contains(got, "--tail=20 ") {
		t.Fatalf("command row after typing 20 = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if got := commandRow(m); got != "kubectl get all -A" {
		t.Fatalf("command row after Esc = %q, want the view's", got)
	}
}

// TestLogsTargetTravelsToTheLogsView: the resolved target goes from the
// container picker through the tail prompt into the logs request, and the
// logs view shows the same selector command.
func TestLogsTargetTravelsToTheLogsView(t *testing.T) {
	t.Parallel()

	target, pods := webTarget(2)
	pick := views.ContainerPromptRequestMsg{
		Namespace: tuitest.NamespaceDemo, Pods: pods, Containers: []string{tuitest.ContainerApp, tuitest.ContainerSidecar},
		NextKind: "logs", Target: webRef(), Logs: target,
	}
	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), pick)
	want := "kubectl logs -f -l 'app=web,pod-template-hash in (h1)' --prefix --tail=100 --max-log-requests=2 -n demo -c app"
	if got := commandRow(m); got != want {
		t.Fatalf("picker command row = %q\nwant                 %q", got, want)
	}

	m, out := tuitest.Drive(t, m, tuitest.Press("2"), clearInput(), tuitest.Type("50"), tuitest.Enter())
	wantReq := views.LogsRequestMsg{
		Namespace: tuitest.NamespaceDemo, Pods: pods, Tail: 50, Container: tuitest.ContainerSidecar,
		Target: webRef(), Logs: target,
	}
	if !tuitest.Contains(out, wantReq) {
		t.Fatalf("want %+v, got:\n%s", wantReq, tuitest.Describe(out))
	}
	want = "kubectl logs -f -l 'app=web,pod-template-hash in (h1)' --prefix --tail=50 --max-log-requests=2 -n demo -c sidecar"
	if got := commandRow(m); got != want {
		t.Fatalf("logs command row = %q\nwant               %q", got, want)
	}
}

// TestDetachedTailPromptSaysSo: logs asked for a row the cursor has left
// open the prompt anyway, marked, and Esc cancels them.
func TestDetachedTailPromptSaysSo(t *testing.T) {
	t.Parallel()

	target, pods := webTarget(2)
	req := views.TailPromptRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: pods, Target: webRef(), Logs: target, Detached: true}
	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), req)
	requireScreen(t, m, "tail lines · deployment/web · ns demo", "(not the selected row)")
	m, out := tuitest.Drive(t, m, tuitest.Esc())
	if logs := tuitest.All[views.LogsRequestMsg](out); len(logs) != 0 {
		t.Fatalf("Esc opened logs: %+v", logs)
	}
	refuteScreen(t, m, "not the selected row")
}

// TestLogsFooterGoldens pins the multi-pod logs footer for a resolved
// deployment: kubectl follows at most 5 pods unless told otherwise.
func TestLogsFooterGoldens(t *testing.T) {
	t.Parallel()

	target, pods := webTarget(15)
	req := views.LogsRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: pods, Tail: 100, Target: webRef(), Logs: target}
	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), req)
	tuitest.Golden(t, "footer_logs_deployment_15_pods", footer(m)+"\n")
}

// TestDetachedFlagTravelsThroughThePicker: a logs request for a row the
// cursor left stays marked from the picker to the tail prompt.
func TestDetachedFlagTravelsThroughThePicker(t *testing.T) {
	t.Parallel()

	target, pods := webTarget(2)
	pick := views.ContainerPromptRequestMsg{
		Namespace: tuitest.NamespaceDemo, Pods: pods, Containers: []string{tuitest.ContainerApp, tuitest.ContainerSidecar},
		NextKind: "logs", Target: webRef(), Logs: target, Detached: true,
	}
	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), pick)
	requireScreen(t, m, "container · deployment/web", "(not the selected row)")
	m, _ = tuitest.Drive(t, m, tuitest.Press("2"))
	requireScreen(t, m, "tail lines · deployment/web · ns demo", "(not the selected row)")
}
