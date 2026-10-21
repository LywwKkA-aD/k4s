package kubectl

import (
	"path/filepath"
	"slices"
)

// Session is what Decorate needs to know about how k4s reached its cluster
// and where a bare kubectl, pasted into the same shell, would go instead.
type Session struct {
	// Context is the kubeconfig context k4s is talking to.
	Context string
	// CurrentContext is the current-context of the kubeconfig k4s loaded.
	CurrentContext string
	// ContextFlag is set when the user started k4s with --context.
	ContextFlag bool
	// Kubeconfig lists the files k4s loaded, in precedence order and
	// preferably absolute, so a pasted command works from any directory.
	Kubeconfig []string
	// BareKubeconfig lists the files a kubectl with no --kubeconfig loads
	// in the same environment; see BareKubeconfig.
	BareKubeconfig []string
	// KubeconfigFlag is set when the user started k4s with --kubeconfig.
	KubeconfigFlag bool
}

// Decorate returns c with the global flags that make a pasted copy reach
// the cluster k4s shows, replacing any c already had:
//
//   - --context when the active context is not the kubeconfig's
//     current-context (after :ctx) or when k4s was started with --context;
//     never on `kubectl config ...`, which works on the file itself;
//   - --kubeconfig, or KUBECONFIG=a:b for several files, when k4s loaded
//     other files than a bare kubectl would (an explicit --kubeconfig, the
//     k3s.yaml fallback, a KUBECONFIG list k4s narrowed) or when k4s was
//     started with --kubeconfig.
func Decorate(c Cmd, s Session) Cmd {
	if c.IsZero() {
		return c.clone()
	}
	ctx := ""
	if needsContext(c, s) {
		ctx = s.Context
	}
	var paths []string
	if needsKubeconfig(s) {
		paths = s.Kubeconfig
	}
	return c.WithContext(ctx).WithKubeconfig(paths)
}

func needsContext(c Cmd, s Session) bool {
	if s.Context == "" || c.Args[0] == "config" {
		return false
	}
	return s.ContextFlag || s.Context != s.CurrentContext
}

func needsKubeconfig(s Session) bool {
	loaded := cleanPaths(s.Kubeconfig)
	if loaded == nil {
		return false
	}
	return s.KubeconfigFlag || !slices.Equal(loaded, cleanPaths(s.BareKubeconfig))
}

// BareKubeconfig returns the files kubectl loads when given no --kubeconfig,
// the way client-go resolves them: a set KUBECONFIG list as-is (empty entries
// and repeats dropped), else ~/.kube/config under home. envValue is the
// value of $KUBECONFIG; home is the user's home directory.
func BareKubeconfig(envValue, home string) []string {
	if envValue == "" {
		if home == "" {
			return nil
		}
		return []string{filepath.Join(home, ".kube", "config")}
	}
	var out []string
	for _, p := range filepath.SplitList(envValue) {
		if p == "" || slices.Contains(out, p) {
			continue
		}
		out = append(out, p)
	}
	return out
}
