package k8s

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"k8s.io/client-go/rest"
)

func TestRingKeepsTheLastLinesWithinItsSize(t *testing.T) {
	t.Parallel()
	r := &ring{}
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 25_000 {
				if _, err := fmt.Fprintf(r, "writer %d: Unable to listen on port %d: bind: address already in use\n", w, i); err != nil {
					t.Errorf("Write: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if _, err := r.Write([]byte("last: lost connection to pod\n")); err != nil {
		t.Fatal(err)
	}

	got := r.String()
	if len(got) > ringSize {
		t.Errorf("ring holds %d bytes after 100k writes, want at most %d", len(got), ringSize)
	}
	if !strings.HasSuffix(got, "last: lost connection to pod\n") {
		t.Errorf("ring lost the newest line: ...%q", got[max(0, len(got)-80):])
	}
	if !strings.HasPrefix(got, "writer ") {
		t.Errorf("ring starts mid-line: %q...", got[:min(len(got), 40)])
	}
}

func TestRingKeepsTheTailOfAnOversizedWrite(t *testing.T) {
	t.Parallel()
	r := &ring{}
	big := strings.Repeat("x", 3*ringSize) + "\nthe end"
	if n, err := r.Write([]byte(big)); n != len(big) || err != nil {
		t.Fatalf("Write = %d, %v; want %d, nil", n, err, len(big))
	}
	if got := r.String(); got != "the end" {
		t.Errorf("ring = %d bytes ending %q, want only the last line", len(got), got[max(0, len(got)-20):])
	}

	small := &ring{}
	if _, err := small.Write([]byte("bind: permission denied")); err != nil {
		t.Fatal(err)
	}
	if got := small.String(); got != "bind: permission denied" {
		t.Errorf("ring = %q", got)
	}
}

func TestPortForwardURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		host string
		want string
	}{
		{host: "https://10.0.0.5:6443", want: "https://10.0.0.5:6443/api/v1/namespaces/demo/pods/web/portforward"},
		{host: "https://rancher.example/k8s/clusters/c-abc", want: "https://rancher.example/k8s/clusters/c-abc/api/v1/namespaces/demo/pods/web/portforward"},
		{host: "http://127.0.0.1:8001", want: "http://127.0.0.1:8001/api/v1/namespaces/demo/pods/web/portforward"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()
			u, err := portForwardURL(&rest.Config{Host: tc.host}, "demo", "web")
			if err != nil {
				t.Fatalf("portForwardURL: %v", err)
			}
			if u.String() != tc.want {
				t.Errorf("URL = %q, want %q", u, tc.want)
			}
		})
	}
}
