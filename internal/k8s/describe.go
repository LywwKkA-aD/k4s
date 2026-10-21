package k8s

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The built-in describer is what k4s shows when `kubectl describe` cannot
// run: kubectl is not on PATH, it failed, or it timed out. It writes a
// subset of kubectl's own layout (the same sections, labels and column
// alignment), so the screen reads the same either way.

// lastAppliedConfigKey is skipped in annotations, as kubectl describe does:
// it holds the whole object again as JSON.
const lastAppliedConfigKey = "kubectl.kubernetes.io/last-applied-configuration"

// maxAnnotationLen is where kubectl describe moves an annotation value to
// its own lines and cuts each of them.
const maxAnnotationLen = 140

// DescribePod describes a pod the way `kubectl describe pod` does: init
// containers and containers with their state and last state, conditions,
// volumes, and the events recorded for this pod's uid.
func (c *Client) DescribePod(ctx context.Context, namespace, name string) (string, error) {
	return c.describePod(ctx, namespace, name, time.Now())
}

// DescribeDeployment describes a deployment the way `kubectl describe
// deployment` does, without its ReplicaSets.
func (c *Client) DescribeDeployment(ctx context.Context, namespace, name string) (string, error) {
	return c.describeDeployment(ctx, namespace, name, time.Now())
}

// DescribeService describes a service the way `kubectl describe service`
// does, without its endpoints.
func (c *Client) DescribeService(ctx context.Context, namespace, name string) (string, error) {
	return c.describeService(ctx, namespace, name, time.Now())
}

// describeWriter writes kubectl describe's layout: two spaces of indent per
// level, cells separated by tabs and aligned by a tabwriter set up as
// kubectl sets up its own.
type describeWriter struct {
	tw *tabwriter.Writer
}

// write prints format at level. Values from the object always go in args,
// never into format.
func (w describeWriter) write(level int, format string, args ...any) {
	_, _ = fmt.Fprint(w.tw, strings.Repeat("  ", level))
	_, _ = fmt.Fprintf(w.tw, format, args...)
}

// flush ends the current block of aligned columns, so the next table
// lines up on its own.
func (w describeWriter) flush() { _ = w.tw.Flush() }

// tabbed runs f against a fresh describeWriter and returns what it wrote.
func tabbed(f func(w describeWriter)) string {
	var buf bytes.Buffer
	w := describeWriter{tw: tabwriter.NewWriter(&buf, 0, 8, 2, ' ', 0)}
	f(w)
	w.flush()
	return buf.String()
}

// stamp formats t as kubectl describe does.
func stamp(t metav1.Time) string {
	return t.Format(time.RFC1123Z)
}

// writeLabels writes a "Title: k=v" block, one pair per line in key order.
// title carries its own indent, as kubectl's does.
func writeLabels(w describeWriter, title string, labels map[string]string) {
	w.write(0, "%s:\t", title)
	keys := sortedKeys(labels, "")
	if len(keys) == 0 {
		w.write(0, "<none>\n")
		return
	}
	for i, k := range keys {
		if i > 0 {
			w.write(0, "\t")
		}
		w.write(0, "%s=%s\n", k, labels[k])
	}
}

// writeAnnotations writes a "Title: k: v" block without
// last-applied-configuration. A long or multi-line value goes on lines of
// its own, each cut at maxAnnotationLen.
func writeAnnotations(w describeWriter, title string, annotations map[string]string) {
	w.write(0, "%s:\t", title)
	keys := sortedKeys(annotations, lastAppliedConfigKey)
	if len(keys) == 0 {
		w.write(0, "<none>\n")
		return
	}
	for i, k := range keys {
		if i > 0 {
			w.write(0, "\t")
		}
		value := strings.TrimSuffix(annotations[k], "\n")
		if len(value)+len(k)+2 <= maxAnnotationLen && !strings.Contains(value, "\n") {
			w.write(0, "%s: %s\n", k, value)
			continue
		}
		w.write(0, "%s:\n", k)
		for line := range strings.SplitSeq(value, "\n") {
			w.write(0, "\t  %s\n", shorten(line, maxAnnotationLen-2))
		}
	}
}

func sortedKeys(m map[string]string, skip string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != skip {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// shorten cuts s to at most limit bytes, on a rune boundary, and marks the
// cut with "...", as kubectl does.
func shorten(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

func orNone(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}
