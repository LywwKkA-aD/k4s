package k8s

import (
	"testing"

	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/version"
)

func TestWithClientDefaults(t *testing.T) {
	t.Parallel()

	in := &rest.Config{Host: "https://10.0.0.1:6443", QPS: 5, Burst: 10, UserAgent: "kubectl/v1.31"}
	got := withClientDefaults(in)

	if got == in {
		t.Fatal("withClientDefaults returned its input; want a copy")
	}
	if got.QPS != clientQPS || got.Burst != clientBurst {
		t.Errorf("QPS/Burst = %v/%d, want %v/%d", got.QPS, got.Burst, clientQPS, clientBurst)
	}
	if want := "k4s/" + version.String(); got.UserAgent != want {
		t.Errorf("UserAgent = %q, want %q", got.UserAgent, want)
	}
	if got.Host != in.Host {
		t.Errorf("Host = %q, want %q", got.Host, in.Host)
	}
	if in.QPS != 5 || in.Burst != 10 || in.UserAgent != "kubectl/v1.31" {
		t.Errorf("input mutated: %+v", in)
	}
}
