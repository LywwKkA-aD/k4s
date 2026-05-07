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
		{"services", "services", true},
		{"svc", "services", true},
		{"service", "services", true},
		{"contexts", "contexts", true},
		{"ctx", "contexts", true},
		{"context", "contexts", true},
		{"top", "top", true},
		{"metrics", "top", true},
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
		"services":    true,
		"contexts":    true,
		"top":         true,
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

func TestAliasesExcludesCanonical(t *testing.T) {
	t.Parallel()

	cases := []struct {
		canonical string
		mustHave  []string
		mustNot   string // canonical name must not appear in its own alias list
	}{
		{canonical: "pods", mustHave: []string{"po", "pod"}, mustNot: "pods"},
		{canonical: "namespaces", mustHave: []string{"ns"}, mustNot: "namespaces"},
		{canonical: "dashboard", mustHave: []string{"home"}, mustNot: "dashboard"},
		{canonical: "deployments", mustHave: []string{"deploy", "deployment", "dp"}, mustNot: "deployments"},
		{canonical: "services", mustHave: []string{"svc", "service"}, mustNot: "services"},
		{canonical: "unknown", mustHave: nil, mustNot: ""},
	}
	for _, tc := range cases {
		got := Aliases(tc.canonical)
		for _, alias := range tc.mustHave {
			if !contains(got, alias) {
				t.Errorf("Aliases(%q) = %q; want to contain %q", tc.canonical, got, alias)
			}
		}
		if tc.mustNot != "" && contains(got, tc.mustNot) {
			t.Errorf("Aliases(%q) = %q; must not contain canonical %q", tc.canonical, got, tc.mustNot)
		}
	}
}

func contains(haystack, needle string) bool {
	for _, p := range splitAndTrim(haystack) {
		if p == needle {
			return true
		}
	}
	return false
}

func splitAndTrim(s string) []string {
	out := []string{}
	current := ""
	for _, r := range s {
		switch r {
		case ',':
			if current != "" {
				out = append(out, current)
				current = ""
			}
		case ' ':
			// skip whitespace between tokens
		default:
			current += string(r)
		}
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}
