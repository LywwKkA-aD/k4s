package forwards

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

const (
	testServer = "https://127.0.0.1:6443"
	testCA     = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	otherCA    = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// thisCluster is the cluster newTestManager serves.
var thisCluster = cluster{contextName: testContext, server: testServer, ca: testCA}

// selfSigned returns a PEM certificate unique to this call.
func selfSigned(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "k3s-server-ca"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestCAFingerprint(t *testing.T) {
	t.Parallel()
	ca, other := selfSigned(t), selfSigned(t)
	file := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(file, append([]byte("\n# k3s server CA\n"), ca...), 0o600); err != nil {
		t.Fatal(err)
	}
	want := caFingerprint(&rest.Config{TLSClientConfig: rest.TLSClientConfig{CAData: ca}})
	if !strings.HasPrefix(want, "sha256:") || len(want) != len("sha256:")+64 {
		t.Fatalf("fingerprint = %q, want sha256:<64 hex>", want)
	}

	cases := []struct {
		name string
		tls  rest.TLSClientConfig
		same bool
		none bool
	}{
		{name: "the same bundle from a file", tls: rest.TLSClientConfig{CAFile: file}, same: true},
		{name: "the same bundle re-wrapped", tls: rest.TLSClientConfig{CAData: append(append([]byte{}, ca...), '\n', '\n')}, same: true},
		{name: "another CA", tls: rest.TLSClientConfig{CAData: other}},
		{name: "no CA", tls: rest.TLSClientConfig{Insecure: true}, none: true},
		{name: "an unreadable file", tls: rest.TLSClientConfig{CAFile: filepath.Join(t.TempDir(), "missing.crt")}, none: true},
		{name: "not PEM", tls: rest.TLSClientConfig{CAData: []byte("garbage")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := caFingerprint(&rest.Config{TLSClientConfig: tc.tls})
			switch {
			case tc.none && got != "":
				t.Errorf("fingerprint = %q, want none", got)
			case tc.same && got != want:
				t.Errorf("fingerprint = %q, want %q", got, want)
			case !tc.same && !tc.none && (got == want || got == ""):
				t.Errorf("fingerprint = %q, want one of its own", got)
			}
		})
	}
}

func TestClusterOf(t *testing.T) {
	t.Parallel()
	ca := selfSigned(t)
	c := &k8s.Client{Context: "default", RestConfig: &rest.Config{Host: testServer, TLSClientConfig: rest.TLSClientConfig{CAData: ca}}}
	got := clusterOf(c)
	want := cluster{contextName: "default", server: testServer, ca: caFingerprint(c.RestConfig)}
	if got != want {
		t.Errorf("clusterOf = %+v, want %+v", got, want)
	}
	if got := clusterOf(&k8s.Client{Context: "fake"}); got != (cluster{contextName: "fake"}) {
		t.Errorf("clusterOf without a rest config = %+v", got)
	}
	if got := clusterOf(nil); got != (cluster{}) {
		t.Errorf("clusterOf(nil) = %+v", got)
	}
}

func TestRelation(t *testing.T) {
	t.Parallel()
	bound := testForward("f", "pod", 1)
	with := func(edit func(*Forward)) Forward {
		f := bound
		edit(&f)
		return f
	}
	cases := []struct {
		name string
		c    cluster
		f    Forward
		want relation
	}{
		{name: "saved here", c: thisCluster, f: bound, want: sameCluster},
		{name: "same name, other host", c: thisCluster, f: with(func(f *Forward) { f.Server = "https://10.0.0.9:6443" }), want: otherCluster},
		{name: "same name and host, other CA", c: thisCluster, f: with(func(f *Forward) { f.CA = otherCA }), want: otherCluster},
		{name: "other context", c: thisCluster, f: with(func(f *Forward) { f.Context = "prod" }), want: otherCluster},
		{name: "legacy, same name", c: thisCluster, f: with(func(f *Forward) { f.Server, f.CA = "", "" }), want: unboundCluster},
		{name: "legacy, no context", c: thisCluster, f: with(func(f *Forward) { f.Context, f.Server, f.CA = "", "", "" }), want: unboundCluster},
		{name: "legacy, other context", c: thisCluster, f: with(func(f *Forward) { f.Context, f.Server, f.CA = "prod", "", "" }), want: otherCluster},
		{name: "no client", c: cluster{}, f: with(func(f *Forward) { f.Context = "prod" }), want: sameCluster},
		{name: "server unknown here", c: cluster{contextName: testContext}, f: with(func(f *Forward) { f.Server, f.CA = "", "" }), want: sameCluster},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.c.relation(tc.f); got != tc.want {
				t.Errorf("relation = %v, want %v", got, tc.want)
			}
		})
	}
}

// seedState writes fwds to a fresh state file the way an earlier k4s left
// it, without going through Register.
func seedState(t *testing.T, fwds ...Forward) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portforwards.json")
	if err := (State{Forwards: fwds}).saveTo(path); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	return path
}

// mixedClusters is one forward per relation to thisCluster.
func mixedClusters() []Forward {
	here := testForward("here", "pod", 1)
	otherHost := testForward("other-host", "pod", 2)
	otherHost.Server = "https://10.0.0.9:6443"
	foreignCA := testForward("other-ca", "pod", 3)
	foreignCA.CA = otherCA
	legacy := unbound(testForward("legacy", "pod", 4))
	contextless := unbound(testForward("contextless", "pod", 5))
	contextless.Context = ""
	prod := testForward("prod", "pod", 6)
	prod.Context = "prod"
	return []Forward{here, otherHost, foreignCA, legacy, contextless, prod}
}

func ids(list []Active) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.Forward.ID)
	}
	slices.Sort(out)
	return out
}

func TestListAndStateKeepToThisCluster(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, deps{statePath: seedState(t, mixedClusters()...)})

	list := m.List()
	if got, want := ids(list), []string{"contextless", "here", "legacy"}; !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
	for _, a := range list {
		if want := a.Forward.ID != "here"; a.Unbound != want {
			t.Errorf("%s: Unbound = %v, want %v", a.Forward.ID, a.Unbound, want)
		}
	}

	// A caller that tells forwards apart by context name would take the
	// same-named forwards of another cluster for this one's.
	saved := m.State().Forwards
	state := make([]string, 0, len(saved))
	for _, f := range saved {
		state = append(state, f.ID)
	}
	slices.Sort(state)
	if want := []string{"contextless", "here", "legacy", "prod"}; !slices.Equal(state, want) {
		t.Errorf("State = %v, want %v", state, want)
	}
}

func TestRegisterBindsToThisCluster(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "portforwards.json")
	m := newTestManager(t, deps{statePath: path})
	mustRegister(t, m, unbound(testForward("f1", "pod", 18080)))

	want := testForward("f1", "pod", 18080)
	if a := statusOf(t, m, "f1"); a.Forward != want || a.Unbound {
		t.Errorf("listed %+v, want %+v, bound", a, want)
	}
	onDisk, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(onDisk.Forwards) != 1 || onDisk.Forwards[0] != want {
		t.Errorf("on disk = %+v, want %+v", onDisk.Forwards, want)
	}
}

func TestStartRefusesAForwardOfAnotherOrUnknownCluster(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	m := newTestManager(t, deps{tunnel: fake, statePath: seedState(t, mixedClusters()...)})

	cases := []struct {
		id   string
		want error
	}{
		{id: "other-host", want: ErrOtherCluster},
		{id: "other-ca", want: ErrOtherCluster},
		{id: "prod", want: ErrOtherCluster},
		{id: "legacy", want: ErrUnbound},
		{id: "contextless", want: ErrUnbound},
	}
	for _, tc := range cases {
		changed := m.Changes()
		if err := m.Start(context.Background(), tc.id); !errors.Is(err, tc.want) {
			t.Errorf("Start(%s) = %v, want %v", tc.id, err, tc.want)
		}
		select {
		case <-changed:
			t.Errorf("a refused Start(%s) changed the forwards", tc.id)
		default:
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.calls) != 0 {
		t.Errorf("refused starts opened tunnels: %+v", fake.calls)
	}
}

func TestReviveBindsALegacyForwardThenStartsIt(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	path := seedState(t, mixedClusters()...)
	m := newTestManager(t, deps{tunnel: fake, statePath: path})

	revived := make(chan error, 1)
	go func() { revived <- m.Revive(context.Background(), "legacy") }()
	fake.next(t).ready()
	if err := awaitErr(t, revived); err != nil {
		t.Fatalf("Revive: %v", err)
	}

	want := testForward("legacy", "pod", 4)
	if a := statusOf(t, m, "legacy"); a.Forward != want || a.Unbound || a.Status != StatusRunning {
		t.Errorf("listed %+v, want %+v running and bound", a, want)
	}
	onDisk, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if got, ok := onDisk.FindByID("legacy"); !ok || got != want {
		t.Errorf("on disk = %+v, want %+v", got, want)
	}
	if err := m.Revive(context.Background(), "other-host"); !errors.Is(err, ErrOtherCluster) {
		t.Errorf("Revive(other-host) = %v, want ErrOtherCluster", err)
	}
	if err := m.Revive(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Revive(missing) = %v, want ErrNotFound", err)
	}
}

func TestWhere(t *testing.T) {
	t.Parallel()
	bound := testForward("f", "pod", 1)
	legacy := unbound(bound)
	contextless := legacy
	contextless.Context = ""
	cases := []struct {
		name string
		a    Active
		want string
	}{
		{name: "bound", a: Active{Forward: bound}, want: "dev · 127.0.0.1:6443"},
		{name: "server without a scheme", a: Active{Forward: Forward{Context: "dev", Server: "10.0.0.5:6443"}}, want: "dev · 10.0.0.5:6443"},
		{name: "server unknown", a: Active{Forward: legacy}, want: "dev"},
		{name: "legacy", a: Active{Forward: legacy, Unbound: true}, want: "dev · unknown cluster, confirm"},
		{name: "legacy without context", a: Active{Forward: contextless, Unbound: true}, want: "unknown cluster, confirm"},
		{name: "control characters", a: Active{Forward: Forward{Context: "dev\x1b[2J", Server: testServer}}, want: "dev · 127.0.0.1:6443"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.a.Where(); got != tc.want {
				t.Errorf("Where = %q, want %q", got, tc.want)
			}
		})
	}
	m := newTestManager(t, deps{})
	if got := m.Where(); got != "dev · 127.0.0.1:6443" {
		t.Errorf("Manager.Where = %q", got)
	}
}
