// Package startup turns the command line into the session the TUI starts
// with: it finds the kubeconfig, applies --context and the starting
// namespace, and runs the credential-plugin preflight. It never draws; main
// owns the terminal.
package startup

import (
	"errors"
	"os"
	"runtime"

	apiversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/util/homedir"

	"github.com/LywwKkA-aD/k4s/internal/cli"
	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// k3sKubeconfig is where a k3s server writes its admin kubeconfig.
const k3sKubeconfig = "/etc/rancher/k3s/k3s.yaml"

// Session is what the TUI starts with.
type Session struct {
	// Client is nil when LoadErr is set. Client.Namespace is the starting
	// namespace ("" for all).
	Client *k8s.Client
	// LoadErr is why there is no Client; the dashboard shows it, and :ctx
	// can still pick another context.
	LoadErr error
	// Source is where the kubeconfig was looked up, also when loading
	// failed.
	Source k8s.Source
	// Distro is the distribution as far as the session knows it: from the
	// context name, and from the server version once Preflight has it. ""
	// without a Client.
	Distro cluster.Distro
	// ServerVersion is set when Preflight ran and the server answered.
	ServerVersion *apiversion.Info
	// PreflightErr is why the preflight failed; the TUI still starts.
	PreflightErr error
}

// Env is what startup reads from the machine.
type Env struct {
	Getenv func(string) string
	// Home is the user's home directory, "" when unknown.
	Home string
	// K3sKubeconfig is the k3s server's kubeconfig, "" where k3s cannot run.
	K3sKubeconfig string
}

// SystemEnv reads Env from this process and machine, finding the home
// directory the way client-go does.
func SystemEnv() Env {
	env := Env{Getenv: os.Getenv, Home: homedir.HomeDir()}
	if runtime.GOOS == "linux" {
		env.K3sKubeconfig = k3sKubeconfig
	}
	return env
}

// Open loads the kubeconfig the command line asks for and picks the
// starting namespace: -A, else -n, else the context's namespace, else all.
// It makes no API call.
//
// A non-nil error means k4s must not start: an explicit --kubeconfig or
// --context cannot be honoured, or a kubeconfig exists but cannot be read
// (the message then carries the fix). Any other load failure lands in
// Session.LoadErr so the TUI can show it.
func Open(opts cli.Options, env Env) (Session, error) {
	explicit := opts.Kubeconfig != "" || opts.Context != ""
	src, err := resolveSource(opts.Kubeconfig, env)
	if err != nil {
		var re *readError
		if explicit || errors.As(err, &re) {
			return Session{}, err
		}
		return Session{Source: src, LoadErr: err}, nil
	}
	client, err := k8s.Load(src, opts.Context)
	if err != nil {
		if explicit {
			return Session{}, err
		}
		return Session{Source: src, LoadErr: err}, nil
	}
	client = client.WithNamespace(startNamespace(opts, client.ContextNamespace))
	return Session{
		Client: client,
		Source: src,
		Distro: cluster.Detect("", client.Context, nil),
	}, nil
}

// startNamespace follows kubectl: -A wins over -n, and -n over the
// context's namespace. Unlike kubectl, a context without a namespace starts
// in all namespaces rather than in "default".
func startNamespace(opts cli.Options, contextNamespace string) string {
	switch {
	case opts.AllNamespaces:
		return ""
	case opts.Namespace != "":
		return opts.Namespace
	}
	return contextNamespace
}
