package k8s

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

// testStartup stands in for portForwardStartup so a stalled upgrade fails
// in well under a second.
const testStartup = 300 * time.Millisecond

// startSlack is how much later than the startup bound the error may land:
// the forwarder goroutine still has to unwind and report.
const startSlack = time.Second

func TestPortForwardStartupIsFifteenSeconds(t *testing.T) {
	t.Parallel()
	if portForwardStartup != 15*time.Second {
		t.Errorf("portForwardStartup = %v, want 15s", portForwardStartup)
	}
}

// silentListener accepts TCP connections and never reads or writes, like
// an apiserver address whose host is up but whose server is wedged. The
// connections stay open until the test ends.
func silentListener(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = l.Close()
		<-done
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return l.Addr().String()
}

// silentTLSServer completes the TLS handshake and reads the upgrade
// request, then never answers it, like a TLS-terminating proxy in front
// of a stuck apiserver.
func silentTLSServer(t *testing.T) *rest.Config {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	srv.StartTLS()
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	return &rest.Config{Host: srv.URL, TLSClientConfig: rest.TLSClientConfig{CAData: ca}}
}

// TestStalledUpgradeFailsWithinTheStartupBound starts a forward against
// servers that never answer the upgrade. Each start must fail within the
// startup bound, leave no goroutine behind and bind no local port.
func TestStalledUpgradeFailsWithinTheStartupBound(t *testing.T) {
	cases := []struct {
		name string
		cfg  func(t *testing.T) *rest.Config
	}{
		{name: "tcp only, plain http", cfg: func(t *testing.T) *rest.Config {
			return &rest.Config{Host: "http://" + silentListener(t)}
		}},
		{name: "tcp only, tls never starts", cfg: func(t *testing.T) *rest.Config {
			return &rest.Config{Host: "https://" + silentListener(t), TLSClientConfig: rest.TLSClientConfig{Insecure: true}}
		}},
		{name: "tls up, upgrade never answered", cfg: silentTLSServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg(t)
			local := freeLocalPort(t)
			baseline := runtime.NumGoroutine()

			c := &Client{Clientset: fake.NewSimpleClientset(), RestConfig: cfg}
			begin := time.Now()
			sess, err := c.startPodPortForward("demo", "web", local, 80, testStartup)
			if err != nil {
				t.Fatalf("startPodPortForward: %v", err)
			}
			requireStartFails(t, sess, testStartup+startSlack)
			if took := time.Since(begin); took > testStartup+startSlack {
				t.Errorf("start failed after %v, want within %v", took, testStartup+startSlack)
			}
			requireGoroutinesBack(t, baseline)
			requirePortFree(t, local)
		})
	}
}

// TestStopAbortsAStalledUpgrade closes the session while the upgrade is
// still pending: the forwarder winds down long before the startup bound.
func TestStopAbortsAStalledUpgrade(t *testing.T) {
	cfg := &rest.Config{Host: "https://" + silentListener(t), TLSClientConfig: rest.TLSClientConfig{Insecure: true}}
	baseline := runtime.NumGoroutine()
	c := &Client{Clientset: fake.NewSimpleClientset(), RestConfig: cfg}
	sess, err := c.startPodPortForward("demo", "web", freeLocalPort(t), 80, time.Minute)
	if err != nil {
		t.Fatalf("startPodPortForward: %v", err)
	}
	sess.Close()
	select {
	case <-sess.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("a stopped session kept waiting for the upgrade")
	}
	requireGoroutinesBack(t, baseline)
}

func TestRefusedUpgradeFallsBackOnlyOnARefusal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "websocket refused", err: &streamhttp.UpgradeFailureError{Cause: errors.New("403 Forbidden")}, want: true},
		{name: "wrapped refusal", err: fmt.Errorf("negotiate: %w", &streamhttp.UpgradeFailureError{Cause: errors.New("400 Bad Request")}), want: true},
		{name: "https proxy", err: errors.New("proxy: unknown scheme: https"), want: true},
		{name: "startup bound", err: context.DeadlineExceeded, want: false},
		{name: "stopped", err: context.Canceled, want: false},
		{name: "unreachable", err: errors.New("dial tcp 10.0.0.1:6443: connect: connection refused"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := refusedUpgrade(tc.err); got != tc.want {
				t.Errorf("refusedUpgrade(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func requireStartFails(t *testing.T, sess *PortForwardSession, within time.Duration) {
	t.Helper()
	select {
	case err := <-sess.Err:
		if err == nil {
			t.Fatal("Err delivered nil")
		}
	case <-sess.Ready:
		t.Fatal("session became ready although the upgrade was never answered")
	case <-time.After(within):
		t.Fatalf("no error within %v", within)
	}
	select {
	case <-sess.Done:
	case <-time.After(within):
		t.Fatal("the forwarder goroutine did not finish")
	}
}

// requireGoroutinesBack waits for the goroutine count to fall back to
// baseline: the dial, the startup watcher and the forwarder must all end.
func requireGoroutinesBack(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > baseline {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			t.Fatalf("goroutines = %d, want at most %d\n%s", runtime.NumGoroutine(), baseline, buf[:n])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func requirePortFree(t *testing.T, port uint16) {
	t.Helper()
	if err := CheckLocalPortAvailable(port); err != nil {
		t.Errorf("local port %d is still bound: %v", port, err)
	}
}

func freeLocalPort(t *testing.T) uint16 {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address is %T", l.Addr())
	}
	if err := l.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("close: %v", err)
	}
	return uint16(addr.Port) //nolint:gosec // a TCP port always fits
}
