package forwards

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// Tunneler is the part of *k8s.Client a Manager drives: it finds the pod
// behind a forward and opens the tunnel to it. WithTunneler swaps it, so a
// test can hand the Manager sessions it controls.
type Tunneler interface {
	StartPodPortForward(ctx context.Context, namespace, podName string, localPort, remotePort uint16) (*k8s.PortForwardSession, error)
	ResolveServiceToPod(ctx context.Context, namespace, serviceName string, svcPort int32) (*corev1.Pod, int32, error)
	ResolveDeploymentToPod(ctx context.Context, namespace, deployName string) (string, error)
}

var _ Tunneler = (*k8s.Client)(nil)

// deps are the Manager's collaborators. NewManager fills them from a
// *k8s.Client and its options; tests swap in fakes and a temporary state
// file.
type deps struct {
	tunnel    Tunneler // nil when k4s runs without a kubeconfig
	cluster   cluster
	probePort func(port uint16) error
	statePath string
	// stopWait bounds every stop's wait for sessions to wind down; zero
	// means stopDeadline.
	stopWait time.Duration
	// after is the previous Manager's stop this one waits for before it
	// registers or starts anything; nil waits for nothing.
	after *Stopping
	// logf writes to the k4s log; nil means klog, which the log sink
	// routes there.
	logf func(format string, args ...any)
}

// Option changes how NewManager builds a Manager.
type Option func(*deps)

// WithTunneler makes the Manager open its tunnels through t instead of the
// client.
func WithTunneler(t Tunneler) Option {
	return func(d *deps) { d.tunnel = t }
}

// After makes the Manager refuse Register and Start with ErrStopping until
// stop has ended, by itself or by timing out. A context switch builds the
// new Manager this way: the old cluster's sessions may still hold the local
// ports a new forward would bind. A nil stop waits for nothing.
func After(stop *Stopping) Option {
	return func(d *deps) { d.after = stop }
}
