package logs_test

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// These tests drive the root model: the way into a crash-looping init
// container crosses the pods view, the container picker and the tail
// prompt, and the footer is the root's.

const broken = "k4s-broken"

// initCrashloop mirrors deploy/seed/70-init-crashloop.yaml: the app waits
// behind two init containers, and migrate keeps failing.
func initCrashloop() []runtime.Object {
	pod := tuitest.Pod(broken, "init-crashloop", map[string]string{"app": "init-crashloop"}, tuitest.ContainerApp)
	pod.Spec.InitContainers = []corev1.Container{{Name: "check-config"}, {Name: "migrate"}}
	return []runtime.Object{tuitest.Namespace(broken), pod}
}

// initLogs serves migrate's log, its previous attempt's, and refuses the
// app container, which never started.
func initLogs() tuitest.FakeOption {
	return tuitest.WithReactor("get", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetSubresource() != "log" {
			return false, nil, nil
		}
		opts, _ := a.(k8stesting.GenericAction).GetValue().(*corev1.PodLogOptions)
		switch {
		case opts == nil || opts.Container != "migrate":
			return true, nil, apierrors.NewBadRequest(`container "app" in pod "init-crashloop" is waiting to start: PodInitializing`)
		case opts.Previous:
			return true, &runtime.Unknown{Raw: []byte("attempt 6: database unreachable\n")}, nil
		}
		return true, &runtime.Unknown{Raw: []byte("running schema migrations...\nmigration failed: database unreachable\n")}, nil
	})
}

func TestInitContainerLogsAreReachable(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithObjects(initCrashloop()...), initLogs())
	m, _ := tuitest.Mount(t, tui.New(c, nil), 160, 30)
	m, _ = tuitest.Drive(t, m, views.NamespaceSelectedMsg{Namespace: broken})

	m, out := tuitest.Drive(t, m, tuitest.Press("l"))
	req, ok := tuitest.Find[views.ContainerPromptRequestMsg](out)
	if !ok {
		t.Fatalf("l on a pod with init containers opened no picker:\n%s", tuitest.Describe(out))
	}
	if want := []string{"app", "check-config", "migrate"}; strings.Join(req.Containers, ",") != strings.Join(want, ",") {
		t.Fatalf("picker lists %v, want %v", req.Containers, want)
	}

	m, out = tuitest.Drive(t, m, tuitest.Press("3"), tuitest.Enter())
	logsReq, ok := tuitest.Find[views.LogsRequestMsg](out)
	if !ok || logsReq.Container != "migrate" {
		t.Fatalf("picking migrate asked for %+v (found %v)", logsReq, ok)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "migration failed: database unreachable") {
		t.Fatalf("the init container's log is not on screen:\n%s", screen)
	}
	tuitest.Golden(t, "root_footer_init_current", tuitest.Footer(m)+"\n")

	m, _ = tuitest.Drive(t, m, tuitest.Press("p"))
	if screen := tuitest.Screen(m); !strings.Contains(screen, "attempt 6: database unreachable") {
		t.Fatalf("p did not show the previous attempt:\n%s", screen)
	}
	tuitest.Golden(t, "root_footer_init_previous", tuitest.Footer(m)+"\n")

	m, _ = tuitest.Drive(t, m, tuitest.Press("p"))
	tuitest.Golden(t, "root_footer_init_current", tuitest.Footer(m)+"\n")
}
