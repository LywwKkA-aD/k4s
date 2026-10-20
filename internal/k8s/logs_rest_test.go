package k8s_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

// logServer answers log requests with two lines and remembers the last
// request it saw.
type logServer struct {
	mu   sync.Mutex
	path string
	q    url.Values
}

func (s *logServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.path, s.q = r.URL.Path, r.URL.Query()
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, "line one\nline two\n")
}

func (s *logServer) last() (string, url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path, s.q
}

type streamFn func(c *k8s.Client, ctx context.Context, ns, pod string, tail int64, container string) (io.ReadCloser, error)

func TestStreamPodLogsRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		stream    streamFn
		tail      int64
		container string
		want      url.Values
	}{
		{
			name: "defaults", stream: (*k8s.Client).StreamPodLogs,
			want: url.Values{"follow": {"true"}},
		},
		{
			name: "tail and container", stream: (*k8s.Client).StreamPodLogs, tail: 50, container: "app",
			want: url.Values{"follow": {"true"}, "tailLines": {"50"}, "container": {"app"}},
		},
		{
			name: "negative tail is ignored", stream: (*k8s.Client).StreamPodLogs, tail: -1,
			want: url.Values{"follow": {"true"}},
		},
		{
			name: "with timestamps", stream: (*k8s.Client).StreamPodLogsWithTimestamps, tail: 10,
			want: url.Values{"follow": {"true"}, "tailLines": {"10"}, "timestamps": {"true"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := &logServer{}
			c := k8stest.NewServerClient(t, srv)

			rc, err := tc.stream(c, context.Background(), "demo", "web", tc.tail, tc.container)
			if err != nil {
				t.Fatalf("stream: %v", err)
			}
			body, err := io.ReadAll(rc)
			if cerr := rc.Close(); cerr != nil {
				t.Fatalf("Close: %v", cerr)
			}
			if err != nil || string(body) != "line one\nline two\n" {
				t.Fatalf("body = %q, err = %v; want both lines verbatim", body, err)
			}
			path, q := srv.last()
			if path != "/api/v1/namespaces/demo/pods/web/log" {
				t.Fatalf("path = %q, want the pod log subresource", path)
			}
			if q.Encode() != tc.want.Encode() {
				t.Fatalf("query = %s, want %s", q.Encode(), tc.want.Encode())
			}
		})
	}
}

func TestStreamPodLogsErrors(t *testing.T) {
	t.Parallel()

	t.Run("forbidden keeps the apiserver's reason", func(t *testing.T) {
		t.Parallel()
		c := k8stest.NewServerClient(t, k8stest.Forbidden())

		rc, err := c.StreamPodLogs(context.Background(), "demo", "web", 10, "")
		if rc != nil {
			t.Fatal("got a stream alongside the error")
		}
		if !apierrors.IsForbidden(err) {
			t.Fatalf("err = %v, want Forbidden", err)
		}
		want := `pods "web" is forbidden: User "` + k8stest.User + `" cannot get resource "pods/log" in API group "" in the namespace "demo"`
		if err.Error() != want {
			t.Fatalf("err = %q, want %q", err, want)
		}
	})

	t.Run("caller deadline cuts a silent server", func(t *testing.T) {
		t.Parallel()
		c := k8stest.NewServerClient(t, k8stest.Hang())
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := c.StreamPodLogs(ctx, "demo", "web", 10, "")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want context.DeadlineExceeded", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("stream setup outlived its deadline: %v", elapsed)
		}
	})

	t.Run("fake clientset outside the granted namespace", func(t *testing.T) {
		t.Parallel()
		c, cs := k8stest.NewFakeClient(clusterObjects()...)
		k8stest.RestrictToNamespace(cs, granted)

		_, err := c.StreamPodLogs(context.Background(), "other", "db", 0, "")
		if !apierrors.IsForbidden(err) || !strings.Contains(err.Error(), `"pods/log"`) {
			t.Fatalf("err = %v, want Forbidden on pods/log", err)
		}
	})
}
