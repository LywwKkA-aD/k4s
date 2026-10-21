package deployments

import (
	"testing"
	"time"
)

// TestListTimeoutGrowsWithTheScope: a list across every namespace may take
// longer than one namespace's before it counts as slow.
func TestListTimeoutGrowsWithTheScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		namespace string
		want      time.Duration
	}{
		{namespace: "", want: 15 * time.Second},
		{namespace: "demo", want: 5 * time.Second},
	}
	for _, tc := range cases {
		t.Run("namespace "+tc.namespace, func(t *testing.T) {
			t.Parallel()
			if got := New(nil, tc.namespace).poll.Timeout(); got != tc.want {
				t.Errorf("timeout = %v, want %v", got, tc.want)
			}
		})
	}
}
