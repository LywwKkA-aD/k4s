package dash

import (
	"slices"
	"testing"
)

func TestCmdStringQuotesForPOSIXShells(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cmd  Cmd
		want string
	}{
		{"plain words", kubectl("get", "pods", "-A"), "kubectl get pods -A"},
		{"glob character", kubectl("get", "--raw=/readyz?verbose"), "kubectl get '--raw=/readyz?verbose'"},
		{"selector with spaces", kubectl("get", "pods", "-l", "app in (a,b)"), "kubectl get pods -l 'app in (a,b)'"},
		{"single quote", kubectl("get", "pod", "it's"), `kubectl get pod 'it'\''s'`},
		{"empty argument", kubectl("get", ""), "kubectl get ''"},
		{"field selector stays bare", kubectl("get", "svc", "--field-selector", "spec.type=LoadBalancer"), "kubectl get svc --field-selector spec.type=LoadBalancer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.cmd.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCmdArgvAndZero(t *testing.T) {
	t.Parallel()

	c := kubectl("top", "nodes")
	if got, want := c.Argv(), []string{"kubectl", "top", "nodes"}; !slices.Equal(got, want) {
		t.Fatalf("Argv() = %q, want %q", got, want)
	}
	argv := c.Argv()
	argv[1] = "delete"
	if c.Args[0] != "top" {
		t.Fatal("Argv shares its backing array with Args")
	}
	if c.IsZero() || !(Cmd{}).IsZero() {
		t.Fatal("IsZero is wrong")
	}
	if got := approx(c); !got.Approx || got.Args[0] != "top" || c.Approx {
		t.Fatalf("approx() = %+v, original %+v", got, c)
	}
}

func TestReadOnlyAllowList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		cmd  Cmd
		want bool
	}{
		{kubectl("get", "pods"), true},
		{kubectl("describe", "pod", "x"), true},
		{kubectl("logs", "x", "--previous"), true},
		{kubectl("top", "nodes"), true},
		{kubectl("events", "-A"), true},
		{kubectl("version"), true},
		{kubectl("cluster-info"), true},
		{kubectl("api-resources"), true},
		{kubectl("auth", "can-i", "--list"), true},
		{kubectl("auth", "whoami"), true},
		{kubectl("auth", "reconcile", "-f", "x"), false},
		{kubectl("auth"), false},
		{kubectl("delete", "pod", "x"), false},
		{kubectl("rollout", "restart", "deploy/x"), false},
		{kubectl("exec", "x", "--", "sh"), false},
		{Cmd{}, false},
	}
	for _, tc := range cases {
		if got := ReadOnly(tc.cmd); got != tc.want {
			t.Errorf("ReadOnly(%q) = %v, want %v", tc.cmd.String(), got, tc.want)
		}
	}
}

func TestScopeArgs(t *testing.T) {
	t.Parallel()

	if got := scopeArgs(""); !slices.Equal(got, []string{"-A"}) {
		t.Fatalf("scopeArgs(\"\") = %q", got)
	}
	if got := scopeArgs("shop"); !slices.Equal(got, []string{"-n", "shop"}) {
		t.Fatalf("scopeArgs(shop) = %q", got)
	}
	if got := canIList("nodes", "", false).String(); got != "kubectl auth can-i list nodes" {
		t.Fatalf("cluster-scoped can-i = %q", got)
	}
	if got := canIList("pods", "", true).String(); got != "kubectl auth can-i list pods -A" {
		t.Fatalf("all-namespaces can-i = %q", got)
	}
	if got := canIList("pods", "shop", true).String(); got != "kubectl auth can-i list pods -n shop" {
		t.Fatalf("namespaced can-i = %q", got)
	}
}
