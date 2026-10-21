package k8s_test

import (
	"context"
	"io"
	"net/url"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

func TestStreamLogsRequest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts k8s.LogOptions
		want url.Values
	}{
		{name: "zero follows the default container", want: url.Values{"follow": {"true"}}},
		{
			name: "tail, container and timestamps",
			opts: k8s.LogOptions{Container: "app", Tail: 250, Timestamps: true},
			want: url.Values{"follow": {"true"}, "tailLines": {"250"}, "container": {"app"}, "timestamps": {"true"}},
		},
		{
			name: "previous instance is read, not followed",
			opts: k8s.LogOptions{Container: "migrate", Tail: 100, Previous: true},
			want: url.Values{"previous": {"true"}, "tailLines": {"100"}, "container": {"migrate"}},
		},
		{name: "no tail sends the whole log", opts: k8s.LogOptions{Tail: -5}, want: url.Values{"follow": {"true"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := &logServer{}
			c := k8stest.NewServerClient(t, srv)

			rc, err := c.StreamLogs(context.Background(), "demo", "web", tc.opts)
			if err != nil {
				t.Fatalf("StreamLogs: %v", err)
			}
			if _, err := io.ReadAll(rc); err != nil {
				t.Fatalf("read: %v", err)
			}
			if err := rc.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			path, q := srv.last()
			if path != "/api/v1/namespaces/demo/pods/web/log" {
				t.Fatalf("path = %q", path)
			}
			if q.Encode() != tc.want.Encode() {
				t.Fatalf("query = %s, want %s", q.Encode(), tc.want.Encode())
			}
		})
	}
}
