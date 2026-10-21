package dash

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
)

const metricsGroupVersion = "metrics.k8s.io/v1beta1"

// call issues one request family. It fills err, rows, bytes and apply.
func (f Fetcher) call(ctx context.Context, r Request, now time.Time) outcome {
	switch r.Endpoint {
	case Pods, Deployments, StatefulSets, DaemonSets, CronJobs, PVCs, Namespaces, HelmCharts, EtcdSnapshots:
		return f.tableCall(ctx, r, now)
	case LoadBalancers:
		return f.loadBalancers(ctx, r)
	case ServiceCount:
		return f.serviceCount(ctx, r)
	case Nodes:
		return f.nodes(ctx, r)
	case Events:
		return f.events(ctx, r)
	case Jobs:
		return f.jobs(ctx, r)
	case NodeMetrics, PodMetrics:
		return f.metrics(ctx, r)
	case Readyz:
		return f.readyz(ctx)
	case Version:
		return f.version(ctx)
	case Whoami:
		return f.whoami(ctx)
	default:
		return f.discovery(ctx)
	}
}

// tablePaths are the list paths of the Table endpoints; {ns} becomes
// namespaces/<ns>/ or nothing.
var tablePaths = map[Endpoint]string{
	Pods:          "/api/v1/{ns}pods",
	Deployments:   "/apis/apps/v1/{ns}deployments",
	StatefulSets:  "/apis/apps/v1/{ns}statefulsets",
	DaemonSets:    "/apis/apps/v1/{ns}daemonsets",
	CronJobs:      "/apis/batch/v1/{ns}cronjobs",
	PVCs:          "/api/v1/{ns}persistentvolumeclaims",
	Namespaces:    "/api/v1/namespaces",
	HelmCharts:    "/apis/helm.cattle.io/v1/{ns}helmcharts",
	EtcdSnapshots: "/apis/k3s.cattle.io/v1/etcdsnapshotfiles",
	LoadBalancers: "/api/v1/{ns}services",
}

func listPath(e Endpoint, ns string) string {
	seg := ""
	if ns != "" {
		seg = "namespaces/" + ns + "/"
	}
	return strings.Replace(tablePaths[e], "{ns}", seg, 1)
}

func (f Fetcher) tableCall(ctx context.Context, r Request, now time.Time) outcome {
	t, n, err := getTable(ctx, f.src.REST, listPath(r.Endpoint, r.Namespace), listOpts{consistent: r.Consistent, limit: r.Limit})
	o := outcome{err: err, bytes: n, rows: t.len() + int(t.remaining)}
	if err != nil {
		return o
	}
	ns := r.Namespace
	switch r.Endpoint {
	case Pods:
		rows, missing := podRowsFrom(t, ns, now), missingColumns(t, podColumns)
		o.apply = func(s *store) { s.pods, s.podsMissing = rows, missing }
	case Deployments:
		rows := deployRowsFrom(t, ns)
		o.apply = func(s *store) { s.deploys = rows }
	case StatefulSets:
		rows := stsRowsFrom(t, ns)
		o.apply = func(s *store) { s.sts = rows }
	case DaemonSets:
		rows := dsRowsFrom(t, ns)
		o.apply = func(s *store) { s.ds = rows }
	case CronJobs:
		rows := cronRowsFrom(t, ns, now)
		o.apply = func(s *store) { s.cronjobs = rows }
	case PVCs:
		rows := statusesFrom(t)
		o.apply = func(s *store) { s.pvcs = rows }
	case Namespaces:
		rows := nsRowsFrom(t, now)
		o.apply = func(s *store) { s.namespaces = rows }
	case HelmCharts:
		rows := helmRowsFrom(t)
		o.apply = func(s *store) { s.helm = rows }
	case EtcdSnapshots:
		rows := snapshotRowsFrom(t, now)
		o.apply = func(s *store) { s.snapshots = rows }
	}
	return o
}

// loadBalancers lists spec.type=LoadBalancer services. A server before
// 1.31 answers 400 to that selector; then, and from then on, the services
// Table is listed and filtered here, which also gives the service count.
func (f Fetcher) loadBalancers(ctx context.Context, r Request) outcome {
	if !r.Fallback {
		l, err := f.src.Typed.CoreV1().Services(r.Namespace).List(ctx, metav1.ListOptions{
			FieldSelector: "spec.type=LoadBalancer", ResourceVersion: rv(r),
		})
		if classify(err) != StateUnsupported {
			o := outcome{err: err}
			if err == nil {
				rows := loadBalancersFrom(l.Items)
				o.rows, o.bytes = len(l.Items), l.Size()
				o.apply = func(s *store) { s.lbs = rows }
			}
			return o
		}
	}
	t, n, err := getTable(ctx, f.src.REST, listPath(LoadBalancers, r.Namespace), listOpts{consistent: r.Consistent})
	o := outcome{err: err, bytes: n, rows: t.len(), requests: 1}
	if !r.Fallback {
		o.requests = 2
	}
	if err == nil {
		rows := serviceRowsFrom(t, r.Namespace)
		o.apply = func(s *store) { s.services, s.lbFallback, s.svcCount, s.svcCountOK = rows, true, len(rows), true }
	}
	return o
}

func (f Fetcher) serviceCount(ctx context.Context, r Request) outcome {
	l, err := f.src.Typed.CoreV1().Services(r.Namespace).List(ctx, metav1.ListOptions{Limit: max(r.Limit, 1)})
	if err != nil {
		return outcome{err: err}
	}
	n := len(l.Items)
	if l.RemainingItemCount != nil {
		n += int(*l.RemainingItemCount)
	}
	return outcome{rows: n, bytes: l.Size(), apply: func(s *store) { s.svcCount, s.svcCountOK = n, true }}
}

func (f Fetcher) nodes(ctx context.Context, r Request) outcome {
	l, err := f.src.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{ResourceVersion: rv(r)})
	if err != nil {
		return outcome{err: err}
	}
	rows := nodesFrom(l.Items)
	return outcome{rows: len(l.Items), bytes: l.Size(), apply: func(s *store) { s.nodes = rows }}
}

func (f Fetcher) events(ctx context.Context, r Request) outcome {
	l, err := f.src.Typed.CoreV1().Events(r.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning", ResourceVersion: rv(r), Limit: r.Limit,
	})
	if err != nil {
		return outcome{err: err}
	}
	rows := eventsFrom(l.Items)
	total, capped := len(rows), l.Continue != ""
	if l.RemainingItemCount != nil {
		total += int(*l.RemainingItemCount)
		capped = true
	}
	// A field selector gets a continue token but no remaining count; the
	// ledger must still read as over the cap or the next refresh lists it all.
	ledgerRows := total
	if capped {
		ledgerRows = max(total, eventCap+1)
	}
	return outcome{rows: ledgerRows, bytes: l.Size(), apply: func(s *store) {
		s.events, s.eventsTotal, s.eventsCapped = rows, total, capped
	}}
}

func (f Fetcher) jobs(ctx context.Context, r Request) outcome {
	l, err := f.src.Typed.BatchV1().Jobs(r.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "status.successful=0", ResourceVersion: rv(r),
	})
	if err != nil {
		return outcome{err: err}
	}
	rows := jobsFrom(l.Items)
	return outcome{rows: len(l.Items), bytes: l.Size(), apply: func(s *store) { s.jobs = rows }}
}

func (f Fetcher) metrics(ctx context.Context, r Request) outcome {
	path := "/apis/" + metricsGroupVersion + "/nodes"
	if r.Endpoint == PodMetrics {
		path = "/apis/" + metricsGroupVersion + "/" + strings.TrimPrefix(listPath(Pods, r.Namespace), "/api/v1/")
	}
	body, err := getRaw(ctx, f.src.REST, path, nil)
	o := outcome{err: err, bytes: len(body)}
	if err != nil {
		return o
	}
	rows, err := metricsFrom(body)
	o.err, o.rows = err, len(rows)
	if r.Endpoint == PodMetrics {
		o.apply = func(s *store) { s.podMetrics = rows }
	} else {
		o.apply = func(s *store) { s.nodeMetrics = rows }
	}
	return o
}

// readyz keeps a failing check (HTTP 500) as data: client-go folds the
// first 2 KiB of the text body into the error, which is enough to name the
// failing checks.
func (f Fetcher) readyz(ctx context.Context) outcome {
	body, err := getRaw(ctx, f.src.REST, "/readyz", map[string]string{"verbose": ""})
	failed := false
	if text, ok := serverText(err); ok {
		body, err, failed = []byte(text), nil, true
	}
	o := outcome{err: err, bytes: len(body)}
	if err != nil {
		return o
	}
	info := parseReadyz(body)
	info.ready = info.ready && !failed
	o.rows, o.apply = info.checks, func(s *store) { s.readyz = info }
	return o
}

// serverText returns the text body of an HTTP 500 that client-go kept.
func serverText(err error) (string, bool) {
	var se *apierrors.StatusError
	if !errors.As(err, &se) || se.ErrStatus.Code != http.StatusInternalServerError || se.ErrStatus.Details == nil {
		return "", false
	}
	for _, c := range se.ErrStatus.Details.Causes {
		if c.Type == metav1.CauseTypeUnexpectedServerResponse && c.Message != "" {
			return c.Message, true
		}
	}
	return "", false
}

func (f Fetcher) version(ctx context.Context) outcome {
	v, err := discovery.ToDiscoveryInterfaceWithContext(f.src.Typed.Discovery()).ServerVersionWithContext(ctx)
	if err != nil {
		return outcome{err: err}
	}
	raw, _ := json.Marshal(v)
	info := versionInfo{git: v.GitVersion, minor: minorOf(v.Minor)}
	old := info.minor > 0 && info.minor < lbFieldSelectorMinor
	return outcome{rows: 1, bytes: len(raw), apply: func(s *store) {
		s.version = info
		s.lbFallback = s.lbFallback || old
	}}
}

// whoami keeps the username only; extra is never read.
func (f Fetcher) whoami(ctx context.Context) outcome {
	rev, err := f.src.Typed.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		return outcome{err: err}
	}
	user := rev.Status.UserInfo.Username
	return outcome{rows: 1, bytes: rev.Size(), apply: func(s *store) { s.user = user }}
}

func (f Fetcher) discovery(ctx context.Context) outcome {
	d := discovery.ToDiscoveryInterfaceWithContext(f.src.Typed.Discovery())
	l, err := d.ServerResourcesForGroupVersionWithContext(ctx, metricsGroupVersion)
	if err != nil {
		return outcome{err: err}
	}
	return outcome{rows: len(l.APIResources), bytes: l.Size(), apply: func(s *store) { s.metricsOK = true }}
}

func rv(r Request) string {
	if r.Consistent {
		return ""
	}
	return "0"
}
