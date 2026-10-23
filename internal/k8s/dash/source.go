package dash

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/logsink"
)

// ErrNoClient reports a missing or empty *k8s.Client.
var ErrNoClient = errors.New("no kubernetes client")

// Source is what the dashboard reads from: Typed for typed lists, REST
// (JSON) for Tables, raw paths and metrics, and the API host from the
// kubeconfig for the server row.
type Source struct {
	Typed kubernetes.Interface
	REST  rest.Interface
	Host  string
}

// NewSource builds the dashboard's clients from c. Typed calls use a
// dashboard-only copy of c's config that speaks protobuf; Tables, CRDs and
// metrics stay JSON through c's own REST client, since a Table over
// protobuf is answered with 406. A client without a RestConfig, or whose
// clientset is not a real one (a fake that only names a server), is used
// as it is.
func NewSource(c *k8s.Client) (Source, error) {
	if c == nil || c.Clientset == nil {
		return Source{}, ErrNoClient
	}
	src := Source{Typed: c.Clientset, REST: c.Clientset.Discovery().RESTClient()}
	if c.RestConfig == nil {
		return src, nil
	}
	src.Host = c.RestConfig.Host
	if _, real := c.Clientset.(*kubernetes.Clientset); !real {
		return src, nil
	}
	cfg := rest.CopyConfig(c.RestConfig)
	cfg.ContentType = runtime.ContentTypeProtobuf
	cfg.AcceptContentTypes = runtime.ContentTypeProtobuf + "," + runtime.ContentTypeJSON
	var typed kubernetes.Interface
	// An exec credential plugin keeps the stderr it sees when the clientset
	// is built; the TUI owns the terminal by now.
	err := logsink.CapturePluginStderr(func() error {
		cs, err := kubernetes.NewForConfig(cfg)
		typed = cs
		return err
	})
	if err != nil {
		return Source{}, fmt.Errorf("build dashboard clientset: %w", err)
	}
	src.Typed = typed
	return src, nil
}
