package dash

import "testing"

// TestExists: whether an object is still in the lists, and whether the
// snapshot can tell at all. A pods Table across every namespace carries no
// namespace, so a pod matches by name there.
func TestExists(t *testing.T) {
	t.Parallel()

	s := refresh(t, newAPIServer(t, busyK3s(fixedNow)), Scope{})
	cases := []struct {
		name          string
		key           ObjectKey
		exists, known bool
	}{
		{name: "a placed pod", key: ObjectKey{Kind: "pod", Namespace: "shop", Name: "payments-worker-6c9f7d8b5-xk2lp"}, exists: true, known: true},
		{name: "a pod by name", key: ObjectKey{Kind: "pod", Name: "payments-worker-6c9f7d8b5-xk2lp"}, exists: true, known: true},
		{name: "a deleted pod", key: ObjectKey{Kind: "pod", Namespace: "shop", Name: "payments-worker-6c9f7d8b5-zzzzz"}, known: true},
		{name: "a node", key: ObjectKey{Kind: "node", Name: "k3s-agent-2"}, exists: true, known: true},
		{name: "a removed node", key: ObjectKey{Kind: "node", Name: "k3s-agent-9"}, known: true},
		{name: "a deployment", key: ObjectKey{Kind: "deployment", Namespace: "shop", Name: "payments-worker"}, exists: true, known: true},
		{name: "a statefulset in another namespace", key: ObjectKey{Kind: "statefulset", Namespace: "staging", Name: "postgres"}, exists: true, known: true},
		{name: "a deleted daemonset", key: ObjectKey{Kind: "daemonset", Namespace: "kube-system", Name: "gone"}, known: true},
		{name: "a kind not listed in full", key: ObjectKey{Kind: "job", Namespace: "ci", Name: "nightly-e2e-28741000"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			exists, known := s.Exists(tc.key)
			if exists != tc.exists || known != tc.known {
				t.Fatalf("Exists(%v) = %v, %v; want %v, %v", tc.key, exists, known, tc.exists, tc.known)
			}
		})
	}
}

// TestExistsNeedsAnAnswer: before a list answers, or once it stopped
// answering, nothing is known to be gone.
func TestExistsNeedsAnAnswer(t *testing.T) {
	t.Parallel()

	k := ObjectKey{Kind: "node", Name: "k3s-agent-2"}
	if _, known := NewSnapshot(Scope{}).Exists(k); known {
		t.Fatal("an empty snapshot claims to know")
	}
	s := refresh(t, newAPIServer(t, busyK3s(fixedNow)), Scope{})
	stale := s
	stale.Ledger[Nodes].State = StateStale
	if _, known := stale.Exists(k); known {
		t.Fatal("a stale list claims to know")
	}
}
