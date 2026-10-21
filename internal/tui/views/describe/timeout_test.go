package describe_test

import (
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
)

// TestBuiltInFallbackHasItsOwnDeadline: on a hung API server the
// built-in describer gives up after its own timeout, not after the time
// kubectl was given as well.
func TestBuiltInFallbackHasItsOwnDeadline(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts []describe.Option
	}{
		{name: "without kubectl"},
		{name: "after kubectl failed", opts: []describe.Option{
			describe.WithKubectl(describe.NewKubectl(kubectl.NewRunner(fakeKubectl(t, "fail", true).cfg), decorate)),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := append([]describe.Option{describe.WithBuiltInTimeout(100 * time.Millisecond)}, tc.opts...)
			m := describe.New(k8stest.NewServerClient(t, k8stest.Hang()), describe.KindPod, tuitest.NamespaceDemo, tuitest.PodWeb1, opts...)
			m, _ = tuitest.Drive(t, m, tuitest.Resize(120, 20))
			m, _ = tuitest.InitWith(t, tuitest.Driver{CmdTimeout: 5 * time.Second}, m)
			if screen := tuitest.Screen(m); !strings.Contains(screen, "describe failed") {
				t.Errorf("the built-in describer did not give up on its own deadline:\n%s", screen)
			}
		})
	}
}
