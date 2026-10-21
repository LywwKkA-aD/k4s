package dash

import "testing"

func TestServiceLBCommandFollowsTheScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		scope    Scope
		fallback bool
		want     string
	}{
		{"all namespaces", Scope{}, false, "kubectl get svc -A --field-selector spec.type=LoadBalancer"},
		{"kube-system", Scope{Namespace: kubeSystem}, false, "kubectl get svc -n kube-system --field-selector spec.type=LoadBalancer"},
		{"old server, all namespaces", Scope{}, true, "kubectl get svc -A"},
		{"old server, kube-system", Scope{Namespace: kubeSystem}, true, "kubectl get svc -n kube-system"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			row := serviceLB(nil, store{lbFallback: tc.fallback}, tc.scope)
			if got := row.Cmd.String(); got != tc.want {
				t.Fatalf("cmd = %q, want %q", got, tc.want)
			}
		})
	}
}
