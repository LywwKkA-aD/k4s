package tui

import (
	"errors"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestExecStatusNamesAreInert: the names in an exec status come from the
// cluster. An escape sequence in one is dropped with its payload, and one
// left open never swallows the rest of the message.
func TestExecStatusNamesAreInert(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	req := views.ExecRequestMsg{Namespace: "demo\x1b[2J", Pod: "web\x1b]0;", Container: "app\x1b]52;c;"}
	missing := errors.New(`exec: "sh": executable file not found in $PATH`)
	cases := []struct {
		name string
		msg  execDoneMsg
		want string
	}{
		{
			name: "the pod is gone",
			msg:  execResolvedMsg{context: c.Context, req: req, err: apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, req.Pod)},
			want: "exec: pod demo/web is gone, press r",
		},
		{
			name: "no shell",
			msg:  shellDoneMsg{req: req, shell: "sh", err: missing},
			want: "no sh in demo/web · y copies the kubectl debug command",
		},
		{
			name: "no shell in a windows pod",
			msg:  shellDoneMsg{req: req, shell: windowsShell, err: missing},
			want: "exec: no cmd.exe in web/app",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			next, _ := New(c, nil).Update(tc.msg)
			if msg, ok := next.(Model).status.Current(time.Now()); !ok || msg.Text != tc.want {
				t.Errorf("status = %q, %v; want %q", msg.Text, ok, tc.want)
			}
		})
	}
}
