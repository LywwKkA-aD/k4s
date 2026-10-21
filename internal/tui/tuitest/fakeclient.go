package tuitest

import (
	"net/http"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// FakeContext is the kubeconfig context name a fake client reports.
const FakeContext = "fake"

// FakeOption customises NewFakeClient.
type FakeOption func(fakeConfig) fakeConfig

type reactor struct {
	verb, resource string
	fn             k8stesting.ReactionFunc
}

type fakeConfig struct {
	context       string
	objects       []runtime.Object
	logs          *string
	metricsStatus int
	metricsOutage *Outage
	reactors      []reactor
}

// WithContext sets the context name the client reports.
func WithContext(name string) FakeOption {
	return func(c fakeConfig) fakeConfig {
		c.context = name
		return c
	}
}

// WithObjects replaces the demo seed with objs. No objects means an empty
// cluster.
func WithObjects(objs ...runtime.Object) FakeOption {
	return func(c fakeConfig) fakeConfig {
		c.objects = append([]runtime.Object(nil), objs...)
		return c
	}
}

// WithLogs sets the body every pod log request streams, then ends. The fake
// API cannot tell pods apart on the log subresource, so all pods share it.
// The default body is client-go's "fake logs".
func WithLogs(body string) FakeOption {
	return func(c fakeConfig) fakeConfig {
		c.logs = &body
		return c
	}
}

// WithMetricsStatus makes the metrics.k8s.io API answer every request with
// status code; http.StatusNotFound looks like a cluster without
// metrics-server.
func WithMetricsStatus(code int) FakeOption {
	return func(c fakeConfig) fakeConfig {
		c.metricsStatus = code
		return c
	}
}

// WithReactor puts fn in front of the fake clientset's reactors for verb on
// resource ("list", "pods"), to inject errors or delays.
func WithReactor(verb, resource string, fn k8stesting.ReactionFunc) FakeOption {
	return func(c fakeConfig) fakeConfig {
		c.reactors = append(append([]reactor(nil), c.reactors...), reactor{verb: verb, resource: resource, fn: fn})
		return c
	}
}

// NewFakeClient returns a k8s.Client backed by fake.NewSimpleClientset and
// seeded with DemoObjects unless WithObjects says otherwise. Its context is
// also the kubeconfig's current-context, as for a plain start. Its discovery
// REST client serves metrics.k8s.io from memory, so the top view works too.
// RestConfig stays nil, which makes port-forward start fail cleanly.
func NewFakeClient(t testing.TB, opts ...FakeOption) *k8s.Client {
	t.Helper()
	cfg := fakeConfig{
		context:       FakeContext,
		objects:       DemoObjects(),
		metricsStatus: http.StatusOK,
	}
	for _, o := range opts {
		cfg = o(cfg)
	}
	cs := fake.NewSimpleClientset(cfg.objects...)
	for _, r := range cfg.reactors {
		cs.PrependReactor(r.verb, r.resource, r.fn)
	}
	if cfg.logs != nil {
		cs.PrependReactor("get", "pods", logsReactor(*cfg.logs))
	}
	metrics, err := newMetricsREST(cfg.metricsStatus, cfg.metricsOutage)
	if err != nil {
		t.Fatalf("build fake metrics client: %v", err)
	}
	fd, ok := cs.Discovery().(*fakediscovery.FakeDiscovery)
	if !ok {
		t.Fatalf("fake clientset discovery is %T", cs.Discovery())
	}
	return &k8s.Client{
		Clientset:      &clientset{Clientset: cs, discovery: &metricsDiscovery{FakeDiscovery: fd, rest: metrics}},
		Context:        cfg.context,
		CurrentContext: cfg.context,
	}
}

// logsReactor answers the pods/log subresource with body.
func logsReactor(body string) k8stesting.ReactionFunc {
	return func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		return true, &runtime.Unknown{Raw: []byte(body)}, nil
	}
}

// clientset swaps in a discovery client with a working RESTClient: the
// stock fake returns nil there, and the metrics code calls through it.
type clientset struct {
	*fake.Clientset
	discovery discovery.DiscoveryInterfaces
}

func (c *clientset) Discovery() discovery.DiscoveryInterfaces { return c.discovery }

type metricsDiscovery struct {
	*fakediscovery.FakeDiscovery
	rest rest.Interface
}

func (d *metricsDiscovery) RESTClient() rest.Interface { return d.rest }
