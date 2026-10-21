package pods

import (
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// TestListTimeoutFollowsTheScope: one namespace answers within 5s; all of
// them get 15s, since a cluster-wide list is the large one (M4.4).
func TestListTimeoutFollowsTheScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		namespace string
		want      time.Duration
	}{
		{namespace: "demo", want: 5 * time.Second},
		{namespace: "", want: 15 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.want.String(), func(t *testing.T) {
			t.Parallel()
			m := New(&k8s.Client{}, tc.namespace)
			if got := m.poll.Timeout(); got != tc.want {
				t.Errorf("Timeout = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestNoClientNeverPolls: without a kubeconfig there is nothing to list,
// so the view starts no load and arms no tick.
func TestNoClientNeverPolls(t *testing.T) {
	t.Parallel()

	m := New(nil, "demo")
	if cmd := m.Init(); cmd != nil {
		t.Error("Init without a client returned a Cmd")
	}
	if m.poll.Enabled() || m.poll.Busy() {
		t.Errorf("poller without a client: enabled=%v busy=%v", m.poll.Enabled(), m.poll.Busy())
	}
}

func TestLayoutSharesTheWidth(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		width         int
		showNamespace bool
		wantNamespace int
		wantName      int
	}{
		{name: "80 one namespace", width: 80, wantName: 30},
		{name: "80 all namespaces", width: 80, showNamespace: true, wantNamespace: 11, wantName: 17},
		{name: "unsized is 80", width: 0, showNamespace: true, wantNamespace: 11, wantName: 17},
		{name: "narrow keeps the minimums", width: 40, showNamespace: true, wantNamespace: minNamespaceWidth, wantName: minNameWidth},
		{name: "wide caps both", width: 300, showNamespace: true, wantNamespace: maxNamespaceWidth, wantName: maxNameWidth},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := layout(tc.width, tc.showNamespace)
			if got.namespace != tc.wantNamespace || got.name != tc.wantName {
				t.Errorf("layout(%d, %v) = %+v, want namespace %d name %d", tc.width, tc.showNamespace, got, tc.wantNamespace, tc.wantName)
			}
		})
	}
}
