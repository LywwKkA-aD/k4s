package command

import "testing"

func TestResolveCanonicalAndAliases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{"pods", "pods", true},
		{"po", "pods", true},
		{"PoD", "pods", true},
		{"  ns  ", "namespaces", true},
		{"namespaces", "namespaces", true},
		{"home", "dashboard", true},
		{"dashboard", "dashboard", true},
		{"deployments", "deployments", true},
		{"deploy", "deployments", true},
		{"dp", "deployments", true},
		{"DP", "deployments", true},
		{"foobar", "", false},
		{"", "", false},
	}

	for _, tc := range cases {
		got, ok := Resolve(tc.input)
		if ok != tc.ok || got != tc.want {
			t.Errorf("Resolve(%q) = %q, %v; want %q, %v", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}

func TestAllReturnsEveryCanonical(t *testing.T) {
	t.Parallel()

	all := All()
	want := map[string]bool{
		"dashboard":   true,
		"pods":        true,
		"namespaces":  true,
		"deployments": true,
	}
	if len(all) != len(want) {
		t.Fatalf("All() = %v; want %d commands", all, len(want))
	}
	for _, c := range all {
		if !want[c] {
			t.Errorf("unexpected canonical %q", c)
		}
	}
}
