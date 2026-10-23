package logs_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
)

// TestTitleNamesWhatIsStreamed: the header and help name the object the
// view streams as the other object views do ("pod · web-1 · yaml"): its
// kind and name, then logs; a deployment's or a service's pods follow,
// by name when there is one. Names from the cluster are made inert.
func TestTitleNamesWhatIsStreamed(t *testing.T) {
	t.Parallel()

	target := func(kind, name string, pods ...string) k8s.LogTarget {
		t := k8s.LogTarget{Kind: kind, Namespace: tuitest.NamespaceDemo, Name: name, Selector: "app=" + name}
		for _, p := range pods {
			t.Pods = append(t.Pods, k8s.PodRef{Name: p})
		}
		return t
	}
	cases := []struct {
		name      string
		pods      []string
		container string
		target    k8s.LogTarget
		want      string
	}{
		{name: "a pod", pods: []string{"web-1"}, want: "pod · web-1 · logs"},
		{name: "a pod's container", pods: []string{"web-1"}, container: "nginx", want: "pod · web-1 / nginx · logs"},
		{
			name: "a deployment's pods", pods: []string{"web-a", "web-b"}, target: target(k8s.LogKindDeployment, "web", "web-a", "web-b"),
			want: "deployment · web · logs · 2 pods",
		},
		{
			name: "a deployment's only pod", pods: []string{"web-a"}, container: "app", target: target(k8s.LogKindDeployment, "web", "web-a"),
			want: "deployment · web · logs · web-a / app",
		},
		{
			name: "a service's pods", pods: []string{"api-1", "api-2", "api-3"}, container: "nginx", target: target(k8s.LogKindService, "api", "api-1", "api-2", "api-3"),
			want: "service · api · logs · 3 pods / nginx",
		},
		{name: "pods without a target", pods: []string{"web-1", "web-2"}, want: "logs · 2 pods"},
		{name: "a hostile pod name", pods: []string{"web\x1b]0;pwned\x07-1"}, container: "ng\x1b[2Jinx", want: "pod · web-1 / nginx · logs"},
		{
			name: "a hostile target name", pods: []string{"web-a", "web-b"}, target: target(k8s.LogKindDeployment, "w\x1b]52;c;eA==\x07eb", "web-a", "web-b"),
			want: "deployment · web · logs · 2 pods",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := logs.New(nil, tuitest.NamespaceDemo, tc.pods, 0, tc.container, logs.WithTarget(tc.target))
			requireTitle(t, m, tc.want)
		})
	}
}
