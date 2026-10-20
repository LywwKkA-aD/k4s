package logsink

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"reflect"
	"runtime"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// logErrorFunc is apimachinery's default ErrorHandler, which logs through
// klog. Install swaps it for one that writes to the Sink directly.
const logErrorFunc = "k8s.io/apimachinery/pkg/util/runtime.logError"

// Install routes klog, apimachinery's HandleError, client-go API warnings
// and the standard log package to s, and turns on exec credential plugin
// stderr capture (see CapturePluginStderr and HoldPluginStderr). It changes
// process-wide state: call it once from main, before the first kubeconfig
// load — a clientset built earlier keeps the real stderr for its plugin —
// and before the TUI starts.
func (s *Sink) Install() error {
	if err := routeKlog(s); err != nil {
		return err
	}
	utilruntime.ErrorHandlers = sinkErrorHandlers(utilruntime.ErrorHandlers, s)
	rest.SetDefaultWarningHandler(rest.NewWarningWriter(s, rest.WarningWriterOptions{Deduplicate: true}))
	log.SetOutput(s)
	pluginStderr.enable()
	return nil
}

// routeKlog configures klog through a private FlagSet so k4s never parses
// klog flags from the command line.
func routeKlog(w io.Writer) error {
	fs := flag.NewFlagSet("klog", flag.ContinueOnError)
	klog.InitFlags(fs)
	settings := []struct{ name, value string }{
		{"logtostderr", "false"},
		{"alsologtostderr", "false"},
		// The default threshold (ERROR) still copies errors to stderr.
		{"stderrthreshold", "FATAL"},
		// SetOutput points every severity at w; without this an error
		// would be written once per severity at or below it.
		{"one_output", "true"},
	}
	for _, f := range settings {
		if err := fs.Set(f.name, f.value); err != nil {
			return fmt.Errorf("set klog %s: %w", f.name, err)
		}
	}
	klog.SetOutput(w)
	return nil
}

// sinkErrorHandlers returns handlers with apimachinery's logError replaced
// by one writing to w. The rest (notably the 1ms backoff that keeps a hot
// error loop from spinning) are kept in order.
func sinkErrorHandlers(handlers []utilruntime.ErrorHandler, w io.Writer) []utilruntime.ErrorHandler {
	out := make([]utilruntime.ErrorHandler, 0, len(handlers)+1)
	out = append(out, errorHandler(w))
	for _, h := range handlers {
		if handlerName(h) != logErrorFunc {
			out = append(out, h)
		}
	}
	return out
}

func handlerName(h utilruntime.ErrorHandler) string {
	fn := runtime.FuncForPC(reflect.ValueOf(h).Pointer())
	if fn == nil {
		return ""
	}
	return fn.Name()
}

func errorHandler(w io.Writer) utilruntime.ErrorHandler {
	return func(_ context.Context, err error, msg string, keysAndValues ...any) {
		line := utilruntime.ErrorToString(err, msg, keysAndValues...)
		_, _ = fmt.Fprintf(w, "%s UnhandledError %s\n", time.Now().Format(time.RFC3339), line)
	}
}
