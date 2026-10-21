package kubectl_test

import (
	"errors"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

func TestHashSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		selector string
		hashes   []string
		want     string
	}{
		{"one hash", "app=growlytics-app", []string{"7d9f8c6b5d"}, "app=growlytics-app,pod-template-hash in (7d9f8c6b5d)"},
		{"rollout keeps newest first", "app=api", []string{"h2", "h1"}, "app=api,pod-template-hash in (h2,h1)"},
		{"match expressions", "app=api,tier in (a,b)", []string{"h1"}, "app=api,tier in (a,b),pod-template-hash in (h1)"},
		{"no hashes leaves the selector alone", "app=api", nil, "app=api"},
		{"empty selector", "", []string{"h1"}, "pod-template-hash in (h1)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := kubectl.HashSelector(tt.selector, tt.hashes); got != tt.want {
				t.Errorf("HashSelector() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectorLogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		spec       kubectl.LogsSpec
		want       string
		wantApprox bool
	}{
		{
			name: "deployment with an evicted pod in its ReplicaSet",
			spec: kubectl.LogsSpec{
				Namespace: "growlytics",
				Selector:  kubectl.HashSelector("app=growlytics-app", []string{"7d9f8c6b5d"}),
				Tail:      100, Matched: 3, Streamed: 2,
			},
			want:       "kubectl logs -f -l 'app=growlytics-app,pod-template-hash in (7d9f8c6b5d)' --prefix --tail=100 --max-log-requests=3 -n growlytics",
			wantApprox: true,
		},
		{
			name: "service streaming 2 of 15 pods",
			spec: kubectl.LogsSpec{Namespace: "growlytics", Selector: "app=growlytics-app", Tail: 50, Matched: 15, Streamed: 2},
			want: "kubectl logs -f -l app=growlytics-app --prefix --tail=50 --max-log-requests=15 -n growlytics",

			wantApprox: true,
		},
		{
			name: "exact set with a container",
			spec: kubectl.LogsSpec{Namespace: "shop", Selector: "app=api", Container: "app", Tail: 10, Matched: 2, Streamed: 2},
			want: "kubectl logs -f -l app=api --prefix --tail=10 --max-log-requests=2 -n shop -c app",
		},
		{
			name: "one pod keeps kubectl's default request cap",
			spec: kubectl.LogsSpec{Namespace: "shop", Selector: "app=api", Tail: 10, Matched: 1, Streamed: 1},
			want: "kubectl logs -f -l app=api --prefix --tail=10 -n shop",
		},
		{
			name: "zero tail asks for the whole log",
			spec: kubectl.LogsSpec{Namespace: "shop", Selector: "app=api", Matched: 2, Streamed: 2},
			want: "kubectl logs -f -l app=api --prefix --tail=-1 --max-log-requests=2 -n shop",
		},
		{
			name: "request cap never undercounts the streamed pods",
			spec: kubectl.LogsSpec{Namespace: "shop", Selector: "app=api", Tail: 10, Matched: 0, Streamed: 7},
			want: "kubectl logs -f -l app=api --prefix --tail=10 --max-log-requests=7 -n shop",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := kubectl.SelectorLogs(tt.spec)
			if err != nil {
				t.Fatalf("SelectorLogs: %v", err)
			}
			if got.String() != tt.want {
				t.Errorf("String() = %q\nwant       %q", got.String(), tt.want)
			}
			if got.Approx != tt.wantApprox {
				t.Errorf("Approx = %v, want %v", got.Approx, tt.wantApprox)
			}
			if got.Mutating {
				t.Error("a logs command is not mutating")
			}
		})
	}
}

func TestSelectorLogsRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec kubectl.LogsSpec
		want error
	}{
		{"missing namespace", kubectl.LogsSpec{Selector: "app=api"}, kubectl.ErrInvalidName},
		{"namespace that reads as a flag", kubectl.LogsSpec{Namespace: "-A", Selector: "app=api"}, kubectl.ErrInvalidName},
		{"bad container", kubectl.LogsSpec{Namespace: "shop", Selector: "app=api", Container: "App_1"}, kubectl.ErrInvalidName},
		{"empty selector", kubectl.LogsSpec{Namespace: "shop"}, kubectl.ErrEmptySelector},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := kubectl.SelectorLogs(tt.spec)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if !got.IsZero() {
				t.Errorf("rejected spec built %q", got.String())
			}
		})
	}
}

func TestPodLogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		namespace, pod, containr string
		tail                     int64
		want                     string
		wantErr                  error
	}{
		{name: "default container", namespace: "shop", pod: "api-7d9f8c6b5d-abcde", tail: 100, want: "kubectl logs -f api-7d9f8c6b5d-abcde --tail=100 -n shop"},
		{name: "named container", namespace: "shop", pod: "api-0", containr: "sidecar", tail: 5, want: "kubectl logs -f api-0 --tail=5 -n shop -c sidecar"},
		{name: "whole log", namespace: "shop", pod: "api-0", want: "kubectl logs -f api-0 --tail=-1 -n shop"},
		{name: "bad pod", namespace: "shop", pod: "--previous", wantErr: kubectl.ErrInvalidName},
		{name: "bad namespace", namespace: "Shop", pod: "api-0", wantErr: kubectl.ErrInvalidName},
		{name: "bad container", namespace: "shop", pod: "api-0", containr: "a.b", wantErr: kubectl.ErrInvalidName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := kubectl.PodLogs(tt.namespace, tt.pod, tt.containr, tt.tail)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got.String() != tt.want {
				t.Errorf("String() = %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestExec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		namespace, pod, containr string
		command                  []string
		want                     string
		wantErr                  error
	}{
		{name: "default container", namespace: "shop", pod: "api-0", command: []string{"sh"}, want: "kubectl exec -it -n shop api-0 -- sh"},
		{name: "named container", namespace: "shop", pod: "api-0", containr: "app", command: []string{"cmd.exe"}, want: "kubectl exec -it -n shop -c app api-0 -- cmd.exe"},
		{name: "command words are quoted", namespace: "shop", pod: "api-0", command: []string{"sh", "-c", "echo $HOME"}, want: "kubectl exec -it -n shop api-0 -- sh -c 'echo $HOME'"},
		{name: "missing command", namespace: "shop", pod: "api-0", wantErr: kubectl.ErrNoCommand},
		{name: "bad pod", namespace: "shop", pod: "API", command: []string{"sh"}, wantErr: kubectl.ErrInvalidName},
		{name: "bad namespace", namespace: "", pod: "api-0", command: []string{"sh"}, wantErr: kubectl.ErrInvalidName},
		{name: "bad container", namespace: "shop", pod: "api-0", containr: "-x", command: []string{"sh"}, wantErr: kubectl.ErrInvalidName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := kubectl.Exec(tt.namespace, tt.pod, tt.containr, tt.command...)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got.String() != tt.want {
				t.Errorf("String() = %q, want %q", got.String(), tt.want)
			}
		})
	}
}
