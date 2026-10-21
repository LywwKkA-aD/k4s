package logs_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
)

// Past 50 pods the picker, the tail prompt and the logs view it opens
// show one command: 50 requests, the default tail, marked approximate.
func TestFooterOverFiftyPodsIsTheSameEverywhere(t *testing.T) {
	t.Parallel()

	target := k8s.LogTarget{Kind: k8s.LogKindService, Namespace: "shop", Name: "web", Selector: "app=web"}
	for i := range 73 {
		target.Pods = append(target.Pods, k8s.PodRef{Name: fmt.Sprintf("web-%02d", i), Containers: []string{"app", "proxy"}})
	}
	pods := target.PodNames()
	ref := views.ResourceRef{Kind: "service", Namespace: "shop", Name: "web"}

	picker := overlay.NewContainer(views.ContainerPromptRequestMsg{
		Namespace: "shop", Pods: pods, Containers: []string{"app", "proxy"}, NextKind: "logs", Target: ref, Logs: target,
	})
	picked, _ := overlay.CommandOf(picker)

	req := views.TailPromptRequestMsg{Namespace: "shop", Pods: pods, Container: "app", Target: ref, Logs: target}
	prompt := overlay.NewTail(req)
	previewed, _ := overlay.CommandOf(prompt)
	_, enter := prompt.Update(tuitest.Enter())
	chosen, ok := enter().(overlay.TailChosenMsg)
	if !ok {
		t.Fatal("Enter chose no tail")
	}
	view := logs.New(nil, "shop", pods, chosen.Tail, "app", logs.WithTarget(target))
	shown := view.Command()

	want := "kubectl logs -f -l app=web --prefix --tail=100 --max-log-requests=50 -n shop -c app"
	for name, c := range map[string]kubectl.Cmd{"picker": picked, "tail prompt": previewed, "logs view": shown} {
		if c.String() != want || !c.Approx {
			t.Errorf("%s shows %q (approx %v)\nwant           %q, approximate", name, c.String(), c.Approx, want)
		}
	}
	if !strings.Contains(prompt.View(80), "max 100 per pod") {
		t.Errorf("the tail prompt names another cap:\n%s", prompt.View(80))
	}
}
