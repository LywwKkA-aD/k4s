package dash

import (
	"testing"
	"time"
)

func TestValidNameKeepsSelectorsSafe(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]bool{
		"payments-worker-6c9f7d8b5-xk2lp":  true,
		"postgres-0":                       true,
		"a,metadata.namespace=kube-system": false,
		"x=y":                              false,
		"":                                 false,
		"UPPER":                            false,
	} {
		if got := validName(name); got != want {
			t.Errorf("validName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestLookupQueue(t *testing.T) {
	t.Parallel()

	crash := func(name, node string) podRow {
		return podRow{name: name, node: node, status: "CrashLoopBackOff", readyOf: 1, readyOK: true, restarts: 3,
			lastRestart: fixedNow.Add(-time.Minute), created: fixedNow.Add(-time.Hour)}
	}
	st := store{pods: []podRow{
		crash("a", "n1"), crash("b", "n1"), crash("a", "n2"),
		{name: "ok", status: "Running", ready: 1, readyOf: 1, readyOK: true},
		{name: "bad,name", status: "Error", readyOf: 1, readyOK: true},
	}, deploys: []workloadRow{{name: "web", ready: 0, desired: 2}, {name: "coredns", ready: 1, desired: 1}}}

	ws := wanted(st, true, fixedNow)
	keys := make([]string, 0, len(ws))
	for _, w := range ws {
		keys = append(keys, w.key.kind+"/"+w.key.name)
	}
	want := []string{"pod/a", "pod/b", "deployment/web", "deployment/coredns"}
	if len(keys) != len(want) {
		t.Fatalf("wanted = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("wanted = %v, want %v", keys, want)
		}
	}
	if ws[0].sig != "n1|2,n2|2" {
		t.Fatalf("pod a signature = %q", ws[0].sig)
	}

	st.resolved = map[resolveKey]resolveEntry{{"pod", "a"}: {sig: ws[0].sig}, {"pod", "gone"}: {}}
	todo := pending(st, ws, 2)
	if len(todo) != 2 || todo[0].key.name != "b" || todo[1].key.name != "web" {
		t.Fatalf("pending = %+v", todo)
	}
	if kept := prune(st.resolved, ws); len(kept) != 1 {
		t.Fatalf("prune kept %v", kept)
	}

	inShop := func(rows []podRow) []podRow {
		out := make([]podRow, len(rows))
		for i, r := range rows {
			r.namespace = "shop"
			out[i] = r
		}
		return out
	}
	scopedDeploys := []workloadRow{{name: "web", namespace: "shop", ready: 0, desired: 2}}
	scoped := wanted(store{pods: inShop(st.pods), deploys: scopedDeploys}, true, fixedNow)
	for _, w := range scoped {
		if w.key.kind != "pod" || w.ns != "shop" {
			t.Fatalf("a namespaced list needs no namespace lookups, only crash containers in its namespace: %+v", w)
		}
	}
}

func TestMatchingRowsToLookups(t *testing.T) {
	t.Parallel()

	objs := []resolvedObj{
		{namespace: "shop", node: "n1", class: ClassOK, restarts: 0},
		{namespace: "staging", node: "n2", class: ClassCrash, restarts: 4},
		{namespace: "dev", node: "n2", class: ClassCrash, restarts: 9},
	}
	row := podRow{name: "postgres-0", node: "n2", restarts: 9}
	used := make([]bool, len(objs))
	if o, i, ok := matchPod(objs, row, ClassCrash, used); !ok || o.namespace != "dev" || i != 2 {
		t.Fatalf("match = %+v %d %v", o, i, ok)
	}
	used[2] = true
	if o, _, ok := matchPod(objs, row, ClassCrash, used); !ok || o.namespace != "staging" {
		t.Fatalf("second match = %+v %v", o, ok)
	}
	if _, _, ok := matchPod(objs, row, ClassCrash, []bool{true, true, true}); ok {
		t.Fatal("matched an object already given to another row")
	}

	ws := []resolvedObj{{namespace: "a", ready: 1, desired: 1}, {namespace: "b", ready: 0, desired: 2}}
	if o, _, ok := matchWorkload(ws, workloadRow{ready: 0, desired: 2}, make([]bool, 2)); !ok || o.namespace != "b" {
		t.Fatalf("workload match = %+v", o)
	}
	if o, _, ok := matchWorkload(ws, workloadRow{ready: 5, desired: 5}, []bool{true, false}); !ok || o.namespace != "b" {
		t.Fatalf("fallback match = %+v", o)
	}
	if _, _, ok := matchWorkload(ws, workloadRow{}, []bool{true, true}); ok {
		t.Fatal("matched with every object used")
	}
}
