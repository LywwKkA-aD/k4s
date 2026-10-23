package dash

import (
	"errors"
	"net/url"
	"slices"
	"syscall"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var versionResource = schema.GroupResource{Resource: "version"}

func refusedVersion() error {
	return &url.Error{Op: "Get", URL: "https://10.0.0.5:6443/version", Err: syscall.ECONNREFUSED}
}

// TestGivenVersionErrorIsRecorded: the caller's /version request failed.
// The ledger records the failure as it would one of the dashboard's own
// requests, so the version row says what a denial says everywhere (— and
// the can-i probe for exactly that request), and the plan still never
// asks /version itself.
func TestGivenVersionErrorIsRecorded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		err   error
		state State
		canI  string
	}{
		{name: "forbidden", err: apierrors.NewForbidden(versionResource, "", errors.New("rbac")), state: StateForbidden, canI: "kubectl auth can-i get /version"},
		{name: "unauthorized", err: apierrors.NewUnauthorized("expired"), state: StateUnauthorized},
		{name: "not found", err: apierrors.NewNotFound(versionResource, ""), state: StateAbsent},
		{name: "server down", err: refusedVersion(), state: StateStale},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := NewSnapshot(Scope{}).GiveVersion("", "").GiveVersionError(tc.err)
			v := s.Cluster.Version
			canI := ""
			if !v.Avail.CanI.IsZero() {
				canI = v.Avail.CanI.String()
			}
			if v.Avail.State != tc.state || canI != tc.canI || v.GitVersion != "" {
				t.Fatalf("version row = %+v; want state %v, can-i %q", v, tc.state, tc.canI)
			}
			if u := s.Ledger[Version]; !u.Fetched || !errors.Is(u.Err, tc.err) {
				t.Fatalf("ledger = %+v, want the caller's error", u)
			}
			if due := dueSet(MakePlan(s.PlanInput(0, true, true, watching))); slices.Contains(due, Version) {
				t.Fatalf("r plans /version after the caller's failed: %v", due)
			}
		})
	}
}

// TestGivenVersionErrorThenVersion: a server down at the start answers the
// caller's retry later, and the answer replaces the failure. Telling the
// same failure again changes nothing.
func TestGivenVersionErrorThenVersion(t *testing.T) {
	t.Parallel()

	down := NewSnapshot(Scope{}).GiveVersion("", "").GiveVersionError(refusedVersion())
	if again := down.GiveVersionError(refusedVersion()); again.Ledger[Version].State != StateStale || again.Hash != down.Hash {
		t.Fatalf("the same failure told twice moved the snapshot: %+v", again.Ledger[Version])
	}
	if same := down.GiveVersionError(nil); same.Ledger != down.Ledger {
		t.Fatal("no error changed the ledger")
	}
	up := down.GiveVersion("v1.36.5+k3s1", "36")
	if v := up.Cluster.Version; v.Avail.State != StateOK || v.GitVersion != "v1.36.5+k3s1" || !up.Caps.K3s {
		t.Fatalf("version after the retry = %+v, k3s %v", v, up.Caps.K3s)
	}
	if err := up.Ledger[Version].Err; err != nil {
		t.Fatalf("the answer kept the failure: %v", err)
	}
}
