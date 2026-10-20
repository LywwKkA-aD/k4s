package logsink

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// restoreGlobals puts back the process-wide logging state Install changes.
func restoreGlobals(t *testing.T) {
	t.Helper()
	handlers := utilruntime.ErrorHandlers
	stdlogOut := log.Writer()
	t.Cleanup(func() {
		utilruntime.ErrorHandlers = handlers
		rest.SetDefaultWarningHandler(rest.WarningLogger{})
		log.SetOutput(stdlogOut)
		_ = pluginStderr.flush(io.Discard)
		pluginStderr.mu.Lock()
		pluginStderr.enabled = false
		pluginStderr.mu.Unlock()

		fs := flag.NewFlagSet("klog-restore", flag.ContinueOnError)
		klog.InitFlags(fs)
		for name, value := range map[string]string{
			"logtostderr":     "true",
			"alsologtostderr": "false",
			"stderrthreshold": "ERROR",
			"one_output":      "false",
		} {
			if err := fs.Set(name, value); err != nil {
				t.Errorf("restore klog %s: %v", name, err)
			}
		}
		klog.SetOutput(os.Stderr)
	})
}

func warningServer(t *testing.T, warning string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Warning", `299 - "`+warning+`"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"major":"1","minor":"31","gitVersion":"v1.31.4"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Install changes process-wide state, so this test does not run in parallel.
func TestInstallKeepsEverythingOffStderr(t *testing.T) {
	restoreGlobals(t)
	srv := warningServer(t, "warn-marker")
	stop := swapStderr(t)

	path := filepath.Join(t.TempDir(), "k4s", "k4s.log")
	s := OpenAt(path, "")
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}

	klog.Error("klog-marker")
	klog.Warning("klog-warning-marker")
	klog.Flush()
	utilruntime.HandleError(errors.New("handle-marker"))
	log.Print("stdlog-marker")

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("NewForConfig: %v", err)
	}
	for range 2 {
		if _, err := cs.Discovery().ServerVersion(); err != nil {
			t.Fatalf("ServerVersion: %v", err)
		}
	}

	terminal := stop()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	logged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	markers := []string{"klog-marker", "klog-warning-marker", "handle-marker", "stdlog-marker", "warn-marker"}
	for _, m := range markers {
		if !strings.Contains(string(logged), m) {
			t.Errorf("log file missing %q:\n%s", m, logged)
		}
		if strings.Contains(terminal, m) {
			t.Errorf("%q reached stderr: %q", m, terminal)
		}
	}
	if terminal != "" {
		t.Errorf("stderr got output: %q", terminal)
	}
	if n := strings.Count(string(logged), "warn-marker"); n != 1 {
		t.Errorf("warning logged %d times, want 1 (deduplicated)", n)
	}
	if n := strings.Count(string(logged), "klog-marker"); n != 1 {
		t.Errorf("klog error logged %d times, want 1 (one_output)", n)
	}
	if got := s.Messages(); got != len(markers) {
		t.Errorf("Messages() = %d, want %d", got, len(markers))
	}
}

func funcName(fn any) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

func TestSinkErrorHandlersReplaceLogErrorKeepBackoff(t *testing.T) {
	t.Parallel()

	defaults := utilruntime.ErrorHandlers
	var out bytes.Buffer
	got := sinkErrorHandlers(defaults, &out)

	if len(got) != len(defaults) {
		t.Fatalf("len = %d, want %d (logError swapped, rest kept)", len(got), len(defaults))
	}
	names := make([]string, 0, len(got))
	for _, h := range got {
		names = append(names, funcName(h))
	}
	joined := strings.Join(names, " ")
	if strings.Contains(joined, logErrorFunc) {
		t.Errorf("logError still installed: %s", joined)
	}
	if !strings.Contains(joined, "backoffError") {
		t.Errorf("rate-limiting handler dropped: %s", joined)
	}
	if funcName(defaults[0]) != logErrorFunc {
		t.Errorf("defaults mutated: first handler is %s", funcName(defaults[0]))
	}

	got[0](context.Background(), errors.New("boom"), "port forward", "port", 8080)
	line := out.String()
	for _, want := range []string{"boom", "port forward", "8080"} {
		if !strings.Contains(line, want) {
			t.Errorf("handler output %q missing %q", line, want)
		}
	}
	if !strings.HasSuffix(line, "\n") {
		t.Errorf("handler output %q should end in a newline", line)
	}
}
