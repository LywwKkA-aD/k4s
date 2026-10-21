package k8s

import (
	"testing"

	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

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

func TestWithStdinUnavailable(t *testing.T) {
	t.Parallel()

	in := &rest.Config{ExecProvider: &clientcmdapi.ExecConfig{Command: "aws", Args: []string{"eks", "get-token"}}}
	got := withStdinUnavailable(in, true, "log in")
	if got.ExecProvider == in.ExecProvider {
		t.Fatal("ExecProvider shared with the input")
	}
	if !got.ExecProvider.StdinUnavailable || got.ExecProvider.StdinUnavailableMessage != "log in" {
		t.Errorf("ExecProvider = %+v, want stdin unavailable with the message", got.ExecProvider)
	}
	if got.ExecProvider.Command != "aws" {
		t.Errorf("Command = %q, want aws", got.ExecProvider.Command)
	}
	if in.ExecProvider.StdinUnavailable || in.ExecProvider.StdinUnavailableMessage != "" {
		t.Errorf("input mutated: %+v", in.ExecProvider)
	}

	plain := withStdinUnavailable(&rest.Config{BearerToken: "t"}, true, "log in")
	if plain.ExecProvider != nil {
		t.Errorf("token config grew an ExecProvider: %+v", plain.ExecProvider)
	}
}

func TestLoginHint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		context string
		src     Source
		want    string
	}{
		{
			name:    "default kubeconfig",
			context: "prod",
			want:    "log in first: run `kubectl --context prod get ns` in a terminal, then press r",
		},
		{
			name:    "flag kubeconfig with a space",
			context: "arn:aws:eks:eu-west-1:1:cluster/prod",
			src:     Source{Paths: []string{"/home/me/my configs/eks"}, Origin: OriginFlag},
			want:    "log in first: run `kubectl --context arn:aws:eks:eu-west-1:1:cluster/prod --kubeconfig '/home/me/my configs/eks' get ns` in a terminal, then press r",
		},
		{
			name:    "quote inside a context name",
			context: "bob's",
			want:    `log in first: run ` + "`" + `kubectl --context 'bob'\''s' get ns` + "`" + ` in a terminal, then press r`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := loginHint(tc.context, tc.src); got != tc.want {
				t.Fatalf("loginHint = %q\nwant        %q", got, tc.want)
			}
		})
	}
}
