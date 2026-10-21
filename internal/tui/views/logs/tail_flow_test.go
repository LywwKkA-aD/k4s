package logs_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
)

// logRequests records the options of every pod log request and answers
// each with body.
type logRequests struct {
	mu   sync.Mutex
	seen []corev1.PodLogOptions
}

func (r *logRequests) reactor(body string) tuitest.FakeOption {
	return tuitest.WithReactor("get", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetSubresource() != "log" {
			return false, nil, nil
		}
		if opts, ok := a.(k8stesting.GenericAction).GetValue().(*corev1.PodLogOptions); ok {
			r.mu.Lock()
			r.seen = append(r.seen, *opts)
			r.mu.Unlock()
		}
		return true, &runtime.Unknown{Raw: []byte(body)}, nil
	})
}

func (r *logRequests) all() []corev1.PodLogOptions {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]corev1.PodLogOptions(nil), r.seen...)
}

func TestTailIsSharedOutAcrossPods(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		pods int
		tail int64
		want int64
	}{
		{name: "twenty pods", pods: 20, tail: 100000, want: 250},
		{name: "one pod", pods: 1, tail: 100000, want: 5000},
		{name: "three pods round up", pods: 3, tail: 9000, want: 1667},
		{name: "a small tail is kept", pods: 20, tail: 40, want: 40},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var reqs logRequests
			c := tuitest.NewFakeClient(t, reqs.reactor("one\n"))
			pods := make([]string, tc.pods)
			for i := range pods {
				pods[i] = fmt.Sprintf("web-%d", i)
			}
			m, _ := tuitest.Mount(t, logs.New(c, tuitest.NamespaceDemo, pods, tc.tail, ""), 100, 20)
			t.Cleanup(func() { _ = m.Close() })

			seen := reqs.all()
			if len(seen) != tc.pods {
				t.Fatalf("%d log requests, want %d", len(seen), tc.pods)
			}
			for _, o := range seen {
				if o.TailLines == nil || *o.TailLines != tc.want {
					t.Fatalf("request tail = %v, want %d", o.TailLines, tc.want)
				}
			}
			if cmd := m.Command().String(); !strings.Contains(cmd, fmt.Sprintf("--tail=%d", tc.want)) {
				t.Errorf("footer command %q does not show the clamped tail", cmd)
			}
		})
	}
}
