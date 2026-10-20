package version

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ldflags string
		info    *debug.BuildInfo
		ok      bool
		want    string
	}{
		{name: "ldflags win", ldflags: "v1.1.5", info: &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}, ok: true, want: "v1.1.5"},
		{name: "go install module version", info: &debug.BuildInfo{Main: debug.Module{Version: "v1.1.5"}}, ok: true, want: "v1.1.5"},
		{name: "local build reports devel", info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, ok: true, want: "dev"},
		{name: "no build info", want: "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolve(tc.ldflags, func() (*debug.BuildInfo, bool) { return tc.info, tc.ok })
			if got != tc.want {
				t.Fatalf("resolve() = %q, want %q", got, tc.want)
			}
		})
	}
}
