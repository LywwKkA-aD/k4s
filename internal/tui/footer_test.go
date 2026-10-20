package tui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestFooterGoldens pins the footer of every view (key hints and the
// kubectl equivalent), ANSI-stripped, at screenW columns. Regenerate with
//
//	go test ./internal/tui -run TestFooterGoldens -update
func TestFooterGoldens(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	logsOne := views.LogsRequestMsg{Namespace: demo, Pods: []string{tuitest.PodWeb1}, Tail: 100}
	cases := []struct {
		name  string
		input []tea.Msg
	}{
		{name: "dashboard"},
		{name: "dashboard_paused", input: tuitest.Keys("w")},
		{name: "pods_all_namespaces", input: colon("pods")},
		{name: "pods_namespace", input: []tea.Msg{views.NamespaceSelectedMsg{Namespace: demo}}},
		{name: "pods_paused", input: then(colon("pods"), tuitest.Keys("w"))},
		{name: "pods_filter_open", input: then(colon("pods"), []tea.Msg{tuitest.Press("/"), tuitest.Type("web")})},
		{name: "deployments", input: colon("deploy")},
		{name: "services", input: colon("svc")},
		{name: "namespaces", input: colon("ns")},
		{name: "contexts", input: colon("ctx")},
		{name: "top_pods", input: colon("top")},
		{name: "top_nodes", input: then(colon("top"), tuitest.Keys("n"))},
		{name: "forwards", input: colon("pf")},
		{name: "describe_pod", input: []tea.Msg{views.DescribeRequestMsg{Kind: "pod", Namespace: demo, Name: tuitest.PodWeb1}}},
		{name: "describe_deployment", input: []tea.Msg{views.DescribeRequestMsg{Kind: "deployment", Namespace: demo, Name: tuitest.DeploymentWeb}}},
		{name: "describe_service", input: []tea.Msg{views.DescribeRequestMsg{Kind: "service", Namespace: demo, Name: tuitest.ServiceWeb}}},
		{name: "logs_single_pod", input: []tea.Msg{logsOne}},
		{name: "logs_single_pod_wrapped", input: []tea.Msg{logsOne, tuitest.Press("w")}},
		{name: "logs_container", input: []tea.Msg{views.LogsRequestMsg{Namespace: demo, Pods: []string{tuitest.PodMulti}, Tail: 20, Container: tuitest.ContainerSidecar}}},
		{name: "logs_multi_pod", input: []tea.Msg{views.LogsRequestMsg{Namespace: demo, Pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}, Tail: 50}}},
		{name: "command_bar", input: tuitest.Keys(":")},
		{name: "command_bar_typing", input: []tea.Msg{tuitest.Press(":"), tuitest.Type("po")}},
		{name: "unknown_command", input: colon("nope")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.input...)
			tuitest.Golden(t, "footer_"+tc.name, footer(m)+"\n")
		})
	}
}
