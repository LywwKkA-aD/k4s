package views_test

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestLogsPromptFor: a resolved target goes to the container picker when
// its pods run several containers, init containers and sidecars counted,
// and to the tail prompt otherwise, naming a single container.
func TestLogsPromptFor(t *testing.T) {
	t.Parallel()

	ref := views.ResourceRef{Kind: "deployment", Namespace: "shop", Name: "web"}
	target := func(containers, init []string) k8s.LogTarget {
		return k8s.LogTarget{Kind: k8s.LogKindDeployment, Namespace: "shop", Name: "web", Pods: podRefs("web-a", "web-b"), Containers: containers, Init: init}
	}
	several := target([]string{"app", "migrate"}, []string{"migrate"})
	one := target([]string{"app"}, nil)
	none := target(nil, nil)
	cases := []struct {
		name     string
		t        k8s.LogTarget
		detached bool
		want     tea.Msg
	}{
		{
			name: "several containers", t: several, detached: true,
			want: views.ContainerPromptRequestMsg{
				Namespace: "shop", Pods: []string{"web-a", "web-b"}, Containers: []string{"app", "migrate"}, Init: []string{"migrate"},
				NextKind: "logs", Target: ref, Logs: several, Detached: true,
			},
		},
		{
			name: "one container", t: one,
			want: views.TailPromptRequestMsg{Namespace: "shop", Pods: []string{"web-a", "web-b"}, Container: "app", Target: ref, Logs: one},
		},
		{
			name: "no container known", t: none,
			want: views.TailPromptRequestMsg{Namespace: "shop", Pods: []string{"web-a", "web-b"}, Target: ref, Logs: none},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := views.LogsPromptFor(ref, tc.t, tc.detached); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("LogsPromptFor =\n%#v\nwant\n%#v", got, tc.want)
			}
		})
	}
}
