package k8s

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// ErrInvalidNamespace reports a name that cannot be a namespace.
var ErrInvalidNamespace = errors.New("invalid namespace")

// ValidateNamespace checks that name can be a namespace: a DNS-1123 label
// of lower-case letters, digits and '-', at most 63 characters. It does not
// ask the cluster, since RBAC may forbid reading namespaces.
func ValidateNamespace(name string) error {
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		return fmt.Errorf("%w %q: %s", ErrInvalidNamespace, name, strings.Join(errs, "; "))
	}
	return nil
}

// Context is a presentation-friendly view of a kubeconfig context — just
// the bits the TUI needs to render a picker (name + cluster + user) and the
// "is this one currently selected?" flag.
type Context struct {
	Name      string
	Cluster   string
	AuthInfo  string
	Namespace string
	Current   bool
}

// ListContexts reads the kubeconfig and returns every defined context.
// Resolution order matches LoadFromKubeconfig (explicit path → $KUBECONFIG
// → ~/.kube/config). The slice preserves whatever order the file uses;
// callers wanting alphabetical output should sort.
func ListContexts(explicitPath string) ([]Context, error) {
	return ListContextsFrom(explicitSource(explicitPath))
}

// ListContextsFrom is ListContexts for the kubeconfig src names, such as a
// running Client's Source.
func ListContextsFrom(src Source) ([]Context, error) {
	raw, err := src.rules().Load()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	out := make([]Context, 0, len(raw.Contexts))
	for name, c := range raw.Contexts {
		out = append(out, Context{
			Name:      name,
			Cluster:   c.Cluster,
			AuthInfo:  c.AuthInfo,
			Namespace: c.Namespace,
			Current:   name == raw.CurrentContext,
		})
	}
	return out, nil
}

// LoadFromKubeconfigContext is the explicit-context cousin of
// LoadFromKubeconfig: same lookup rules, but the named context wins over
// whatever the file's current-context says. Empty contextName falls back
// to the current-context to keep behaviour identical to LoadFromKubeconfig.
func LoadFromKubeconfigContext(explicitPath, contextName string) (*Client, error) {
	return Load(explicitSource(explicitPath), contextName)
}

// explicitSource is the Source for a --kubeconfig-style path, or the
// default lookup when path is "".
func explicitSource(path string) Source {
	if path == "" {
		return Source{}
	}
	return Source{Paths: []string{path}, Origin: OriginFlag}
}
