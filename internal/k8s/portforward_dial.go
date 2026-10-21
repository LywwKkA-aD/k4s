package k8s

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

// portForwardStartup bounds how long a forward may take to reach the API
// server and upgrade its connection: the TCP connect, the TLS handshake
// and the wait for the upgrade response. kubectl waits forever; a TUI
// whose forward row says "starting" must not.
const portForwardStartup = 15 * time.Second

// startupContext ends after d, or as soon as stopCh closes so a stopped
// forward abandons its dial at once.
func startupContext(stopCh <-chan struct{}, d time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// withStartupContext returns a copy of cfg whose transports send every
// request under ctx. Neither upgrade transport reads rest.Config.Dial; both
// take their deadline from the request context: the WebSocket dialer for
// the whole handshake, the SPDY one for the connect and the TLS handshake.
func withStartupContext(ctx context.Context, cfg *rest.Config) *rest.Config {
	out := rest.CopyConfig(cfg)
	out.Wrap(func(next http.RoundTripper) http.RoundTripper {
		return contextRoundTripper{next: next, ctx: ctx}
	})
	return out
}

// contextRoundTripper sends each request under ctx. The upgrade dialers
// build their requests without one.
type contextRoundTripper struct {
	next http.RoundTripper
	ctx  context.Context
}

// RoundTrip implements http.RoundTripper.
func (c contextRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return c.next.RoundTrip(req.WithContext(c.ctx))
}

// portForwardURL is the pod's portforward subresource, built the way
// kubectl builds it: through a REST client, under any path prefix in the
// server address (Rancher serves clusters at /k8s/clusters/<id>), with the
// namespace and pod name escaped as path segments.
func portForwardURL(cfg *rest.Config, namespace, pod string) (*url.URL, error) {
	core, err := corev1client.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build rest client: %w", err)
	}
	return core.RESTClient().Post().Resource("pods").Namespace(namespace).Name(pod).SubResource("portforward").URL(), nil
}

// newPortForwardDialer is kubectl's port-forward transport: SPDY tunnelled
// over a WebSocket first, plain SPDY when the server or a proxy refuses the
// WebSocket upgrade (API servers before 1.30 do).
func newPortForwardDialer(cfg *rest.Config, u *url.URL) (streamhttp.Dialer, error) {
	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("build spdy transport: %w", err)
	}
	spdyDialer := spdy.NewDialerForStreaming(upgrader, &http.Client{Transport: transport}, http.MethodPost, u)
	wsDialer, err := portforward.NewSPDYOverWebsocketDialerForStreaming(u, cfg)
	if err != nil {
		return nil, fmt.Errorf("build websocket transport: %w", err)
	}
	return portforward.NewFallbackDialerForStreaming(wsDialer, spdyDialer, refusedUpgrade), nil
}

// refusedUpgrade reports whether the WebSocket dial failed because the
// server or a proxy answered the upgrade with a refusal, the case kubectl
// retries over SPDY. A dial that timed out or was stopped does not
// qualify.
func refusedUpgrade(err error) bool {
	return streamhttp.IsUpgradeFailure(err) || streamhttp.IsHTTPSProxyError(err)
}
