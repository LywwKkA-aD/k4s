package logs_test

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
)

// instances answers the current and the previous instance's log with
// their own bodies; a nil previous error means the container restarted.
type instances struct {
	current, previous string
	prevErr           error
	prevAsked         atomic.Int32
}

func (in *instances) option() tuitest.FakeOption {
	return tuitest.WithReactor("get", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetSubresource() != "log" {
			return false, nil, nil
		}
		opts, _ := a.(k8stesting.GenericAction).GetValue().(*corev1.PodLogOptions)
		if opts == nil || !opts.Previous {
			return true, &runtime.Unknown{Raw: []byte(in.current)}, nil
		}
		in.prevAsked.Add(1)
		if in.prevErr != nil {
			return true, nil, in.prevErr
		}
		return true, &runtime.Unknown{Raw: []byte(in.previous)}, nil
	})
}

func noPrevious(container, pod string) error {
	return apierrors.NewBadRequest(`previous terminated container "` + container + `" in pod "` + pod + `" not found`)
}

func mountInstances(t *testing.T, in *instances, container string, pods ...string) logs.Model {
	t.Helper()
	c := tuitest.NewFakeClient(t, in.option())
	m, _ := tuitest.Mount(t, logs.New(c, tuitest.NamespaceDemo, pods, 100, container), 100, 20)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestPreviousTogglesTheInstance(t *testing.T) {
	t.Parallel()

	in := &instances{current: "serving\n", previous: "panic: boom\n"}
	m := mountInstances(t, in, tuitest.ContainerNginx, tuitest.PodWeb1)
	steps := []struct {
		title, command, shows, hides string
	}{
		{
			title:   "logs · web-1 / nginx",
			command: "kubectl logs -f web-1 --tail=100 -n demo -c nginx",
			shows:   "serving", hides: "panic: boom",
		},
		{
			title:   "logs · web-1 / nginx · previous",
			command: "kubectl logs web-1 --tail=100 -n demo -c nginx --previous",
			shows:   "panic: boom", hides: "serving",
		},
		{
			title:   "logs · web-1 / nginx",
			command: "kubectl logs -f web-1 --tail=100 -n demo -c nginx",
			shows:   "serving", hides: "panic: boom",
		},
	}
	for i, s := range steps {
		if i > 0 {
			m, _ = tuitest.Drive(t, m, tuitest.Press("p"))
		}
		requireTitle(t, m, s.title)
		if got := m.Command(); got.String() != s.command || got.Approx {
			t.Errorf("step %d: command = %q (approx %v), want %q", i, got.String(), got.Approx, s.command)
		}
		screen := tuitest.Screen(m)
		if !strings.Contains(screen, s.shows) || strings.Contains(screen, s.hides) {
			t.Errorf("step %d: want %q and not %q:\n%s", i, s.shows, s.hides, screen)
		}
	}
	if got := in.prevAsked.Load(); got != 1 {
		t.Errorf("previous instance asked %d times, want 1", got)
	}
}

func TestPreviousWithoutOneSaysWhy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		container string
		err       error
		level     status.Level
		text      string
	}{
		{
			name: "not restarted", container: tuitest.ContainerNginx, err: noPrevious(tuitest.ContainerNginx, tuitest.PodWeb1),
			level: status.Warn, text: "container nginx of pod web-1 has no previous instance: it has not restarted",
		},
		{
			name: "default container", err: noPrevious(tuitest.ContainerNginx, tuitest.PodWeb1),
			level: status.Warn, text: "pod web-1 has no previous instance: its container has not restarted",
		},
		{
			name: "pod gone", err: apierrors.NewNotFound(corev1.Resource("pods"), tuitest.PodWeb1),
			level: status.Warn, text: "pod web-1 is gone",
		},
		{
			name: "refused by RBAC", err: logsForbidden(tuitest.NamespaceDemo, tuitest.PodWeb1),
			level: status.Error, text: "previous logs of pod web-1: forbidden: get pods/log in demo · kubectl auth can-i get pods --subresource=log -n demo",
		},
		{
			name: "refused without the API's words", err: apierrors.NewForbidden(corev1.Resource("pods/log"), tuitest.PodWeb1, nil),
			level: status.Error, text: "previous logs of pod web-1: forbidden: pods/log · kubectl auth can-i --list",
		},
		{
			name: "credentials expired", err: apierrors.NewUnauthorized("token expired"),
			level: status.Error, text: "previous logs of pod web-1: unauthorized: credentials expired or revoked",
		},
		{
			name: "anything else", err: apierrors.NewInternalError(errBoom),
			level: status.Error, text: "previous logs of pod web-1: Internal error occurred: boom",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := &instances{current: "serving\n", prevErr: tc.err}
			m := mountInstances(t, in, tc.container, tuitest.PodWeb1)
			m, out := tuitest.Drive(t, m, tuitest.Press("p"))
			got, ok := tuitest.Find[views.StatusMsg](out)
			if !ok || got.Level != tc.level || got.Text != tc.text {
				t.Fatalf("status = %+v (found %v), want level %v %q", got, ok, tc.level, tc.text)
			}
			if strings.Contains(m.Title(), "previous") || !strings.Contains(tuitest.Screen(m), "serving") {
				t.Errorf("a failed p left the current stream:\ntitle %q\n%s", m.Title(), tuitest.Screen(m))
			}
		})
	}
}

// logsForbidden is RBAC refusing pod's log as the API server words it.
func logsForbidden(ns, pod string) error {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure, Code: http.StatusForbidden, Reason: metav1.StatusReasonForbidden,
		Message: `pods "` + pod + `" is forbidden: User "dev" cannot get resource "pods/log" in API group "" in the namespace "` + ns + `"`,
	}}
}

type boomError struct{}

func (boomError) Error() string { return "boom" }

var errBoom = boomError{}

func TestPreviousNeedsOnePod(t *testing.T) {
	t.Parallel()

	in := &instances{current: "serving\n", previous: "old\n"}
	m := mountInstances(t, in, "", tuitest.PodWeb1, tuitest.PodWeb2)
	_, out := tuitest.Drive(t, m, tuitest.Press("p"))
	got, ok := tuitest.Find[views.StatusMsg](out)
	if !ok || got.Level != status.Warn || !strings.Contains(got.Text, "one pod") {
		t.Fatalf("status = %+v (found %v), want a warning that p needs one pod", got, ok)
	}
	if in.prevAsked.Load() != 0 {
		t.Error("a multi-pod view asked for a previous instance")
	}
	for _, b := range m.Help() {
		if b.Help().Key == "p" {
			t.Error("a multi-pod view advertises p")
		}
	}
}

func TestPreviousIsAdvertisedForOnePod(t *testing.T) {
	t.Parallel()

	in := &instances{current: "serving\n", previous: "old\n"}
	m := mountInstances(t, in, "", tuitest.PodWeb1)
	want := map[string]string{"p": "previous"}
	for range 2 {
		found := ""
		for _, b := range m.Help() {
			if b.Help().Key == "p" {
				found = b.Help().Desc
			}
		}
		if found != want["p"] {
			t.Fatalf("p is advertised as %q, want %q", found, want["p"])
		}
		m, _ = tuitest.Drive(t, m, tuitest.Press("p"))
		want["p"] = "current"
	}
}

func TestPreviousOfflineSaysSo(t *testing.T) {
	t.Parallel()

	m := logs.New(nil, tuitest.NamespaceDemo, []string{tuitest.PodWeb1}, 100, "")
	_, out := tuitest.Drive(t, m, tuitest.Press("p"))
	if got, ok := tuitest.Find[views.StatusMsg](out); !ok || got.Text != "no kubeconfig" {
		t.Fatalf("status = %+v (found %v), want no kubeconfig", got, ok)
	}
}
