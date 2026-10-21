package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestParsePodList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    podSet
		wantErr string
	}{
		{
			name: "custom columns",
			in: "k4s-demo   nginx-7f6d9c4b8-2mzqt   1201\n" +
				"kube-system   coredns-5d78c9869d-x7k2p   988\n",
			want: podSet{"k4s-demo/nginx-7f6d9c4b8-2mzqt": "1201", "kube-system/coredns-5d78c9869d-x7k2p": "988"},
		},
		{name: "no pods", in: "", want: podSet{}},
		{name: "blank lines", in: "\n a b 1 \n\n", want: podSet{"a/b": "1"}},
		{name: "missing column", in: "k4s-demo nginx\n", wantErr: "want namespace, name and resourceVersion"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parsePodList([]byte(tc.in))
			checkErr(t, err, tc.wantErr)
			if tc.wantErr == "" && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("pods = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDiffPods(t *testing.T) {
	t.Parallel()

	before := podSet{
		"k4s-demo/nginx-a":            "10",
		"k4s-demo/heartbeat-1-abcde":  "20",
		"k4s-broken/crash-loop-xyz":   "30",
		"kube-system/coredns-1-q9w8e": "40",
	}
	after := podSet{
		"k4s-demo/nginx-a":            "10",
		"k4s-demo/heartbeat-2-fghij":  "55",
		"k4s-broken/crash-loop-xyz":   "61",
		"kube-system/coredns-1-q9w8e": "40",
	}

	cases := []struct {
		name          string
		before, after podSet
		want          churn
	}{
		{name: "idle", before: before, after: before, want: churn{}},
		{
			name: "cronjob and crash loop",
			want: churn{
				added: 1, removed: 1, updated: 1,
				byNamespace: []nsCount{{ns: "k4s-demo", n: 2}, {ns: "k4s-broken", n: 1}},
			},
			before: before,
			after:  after,
		},
		{
			name:   "ties sort by name",
			before: podSet{},
			after:  podSet{"b/x": "1", "a/y": "1"},
			want:   churn{added: 2, byNamespace: []nsCount{{ns: "a", n: 1}, {ns: "b", n: 1}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := diffPods(tc.before, tc.after)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("diffPods = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestChurnTotal(t *testing.T) {
	t.Parallel()
	if got := (churn{added: 2, removed: 1, updated: 4}).total(); got != 7 {
		t.Fatalf("total = %d, want 7", got)
	}
}

func TestCompareSnapshots(t *testing.T) {
	t.Parallel()

	errDown := errors.New("connection refused")
	cases := []struct {
		name      string
		before    podSet
		beforeErr error
		after     podSet
		afterErr  error
		wantOK    bool
		wantErr   string
		wantTotal int
	}{
		{name: "both lists", before: podSet{"a/b": "1"}, after: podSet{"a/b": "2"}, wantOK: true, wantTotal: 1},
		{name: "before failed", beforeErr: errDown, wantErr: "before the window: connection refused"},
		{name: "after failed", before: podSet{}, afterErr: errDown, wantErr: "after the window: connection refused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := compareSnapshots(tc.before, tc.beforeErr, tc.after, tc.afterErr)
			if got.checked != tc.wantOK {
				t.Fatalf("checked = %v, want %v", got.checked, tc.wantOK)
			}
			if got.err != tc.wantErr {
				t.Fatalf("err = %q, want %q", got.err, tc.wantErr)
			}
			if got.churn.total() != tc.wantTotal {
				t.Fatalf("total = %d, want %d", got.churn.total(), tc.wantTotal)
			}
		})
	}
}

func TestKubectlArgs(t *testing.T) {
	t.Parallel()

	const cols = "custom-columns=NS:.metadata.namespace,NAME:.metadata.name,RV:.metadata.resourceVersion"
	cases := []struct {
		name       string
		kubeconfig string
		want       []string
	}{
		{name: "inherited kubeconfig", want: []string{"get", "pods", "-A", "--no-headers", "-o", cols}},
		{
			name:       "explicit kubeconfig",
			kubeconfig: ".kube/config",
			want:       []string{"--kubeconfig", ".kube/config", "get", "pods", "-A", "--no-headers", "-o", cols},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := kubectlPodArgs(tc.kubeconfig); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("args = %q, want %q", got, tc.want)
			}
		})
	}
}
