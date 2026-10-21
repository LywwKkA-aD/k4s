package k8s_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

type describer struct {
	kind     string
	prefix   string // wrap on a failed Get
	describe func(*k8s.Client, context.Context, string, string) (string, error)
}

func describers() []describer {
	return []describer{
		{kind: "Pod", prefix: "get pod: ", describe: (*k8s.Client).DescribePod},
		{kind: "Deployment", prefix: "get deployment: ", describe: (*k8s.Client).DescribeDeployment},
		{kind: "Service", prefix: "get service: ", describe: (*k8s.Client).DescribeService},
	}
}

func describeObjects(kind string) []runtime.Object {
	meta := metav1.ObjectMeta{Name: "web", Namespace: granted}
	return []runtime.Object{
		&corev1.Pod{ObjectMeta: meta},
		&appsv1.Deployment{ObjectMeta: meta},
		&corev1.Service{ObjectMeta: meta},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "other"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "other"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "other"}},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "web.1", Namespace: granted},
			InvolvedObject: corev1.ObjectReference{Kind: kind, Name: "web", Namespace: granted},
			Type:           corev1.EventTypeWarning,
			Reason:         "Probe",
			Message:        "readiness probe failed",
		},
	}
}

// TestDescribeSurvivesAFailedEventsList pins that events are best-effort:
// when listing them fails, describe still renders the object and says
// the events are unavailable with a short reason, never the raw error,
// which names the user and the verb.
func TestDescribeSurvivesAFailedEventsList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		install    func(*fake.Clientset)
		wantEvents bool
		wantLine   string
	}{
		{name: "events readable", install: func(*fake.Clientset) {}, wantEvents: true},
		{
			name:     "events forbidden",
			install:  func(cs *fake.Clientset) { k8stest.Forbid(cs, "list", "events") },
			wantLine: "<unavailable: forbidden>",
		},
		{
			name: "events list unauthorized",
			install: func(cs *fake.Clientset) {
				cs.PrependReactor("list", "events", func(ktesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewUnauthorized("Unauthorized")
				})
			},
			wantLine: "<unavailable: unauthorized>",
		},
		{
			name: "events list fails otherwise",
			install: func(cs *fake.Clientset) {
				cs.PrependReactor("list", "events", func(ktesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewInternalError(errors.New("etcd is sad"))
				})
			},
			wantLine: "<unavailable: error>",
		},
		{
			name: "events list times out",
			install: func(cs *fake.Clientset) {
				cs.PrependReactor("list", "events", func(ktesting.Action) (bool, runtime.Object, error) {
					return true, nil, context.DeadlineExceeded
				})
			},
			wantLine: "<unavailable: timeout>",
		},
	}
	for _, d := range describers() {
		for _, tc := range cases {
			t.Run(d.kind+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				c, cs := k8stest.NewFakeClient(describeObjects(d.kind)...)
				tc.install(cs)

				out, err := d.describe(c, context.Background(), granted, "web")
				if err != nil {
					t.Fatalf("Describe%s: %v", d.kind, err)
				}
				if !strings.Contains(out, "Name:") || !strings.Contains(out, "web") {
					t.Fatalf("Describe%s output lacks the object header:\n%s", d.kind, out)
				}
				hasEvents := strings.Contains(out, "Events:") && strings.Contains(out, "readiness probe failed")
				if hasEvents != tc.wantEvents {
					t.Fatalf("Events section shown = %v, want %v:\n%s", hasEvents, tc.wantEvents, out)
				}
				if tc.wantLine != "" && !hasLine(out, "Events:", tc.wantLine) {
					t.Fatalf("Describe%s lacks %q:\n%s", d.kind, tc.wantLine, out)
				}
				for _, leak := range []string{"cannot list", "Unauthorized", "etcd"} {
					if strings.Contains(out, leak) {
						t.Fatalf("Describe%s leaked the events error into the output:\n%s", d.kind, out)
					}
				}
			})
		}
	}
}

// hasLine reports a line of out that starts with prefix and ends with
// suffix, whatever the alignment between them.
func hasLine(out, prefix, suffix string) bool {
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, suffix) {
			return true
		}
	}
	return false
}

func TestDescribeGetErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		install   func(*fake.Clientset)
		namespace string
		is        func(error) bool
	}{
		{
			name:      "outside the granted namespace",
			install:   func(cs *fake.Clientset) { k8stest.RestrictToNamespace(cs, granted) },
			namespace: "other",
			is:        apierrors.IsForbidden,
		},
		{
			name:      "revoked credentials",
			install:   func(cs *fake.Clientset) { k8stest.Unauthorized(cs) },
			namespace: granted,
			is:        apierrors.IsUnauthorized,
		},
		{
			name:      "object gone",
			install:   func(*fake.Clientset) {},
			namespace: "missing",
			is:        apierrors.IsNotFound,
		},
	}
	for _, d := range describers() {
		for _, tc := range cases {
			t.Run(d.kind+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				c, cs := k8stest.NewFakeClient(describeObjects(d.kind)...)
				tc.install(cs)

				out, err := d.describe(c, context.Background(), tc.namespace, "web")
				if !tc.is(err) {
					t.Fatalf("Describe%s err = %v, want it classified as %s", d.kind, err, tc.name)
				}
				if !strings.HasPrefix(err.Error(), d.prefix) {
					t.Fatalf("Describe%s err = %q, want prefix %q", d.kind, err, d.prefix)
				}
				if out != "" {
					t.Fatalf("Describe%s returned output alongside the error:\n%s", d.kind, out)
				}
			})
		}
	}
}

func TestDescribeInsideTheGrantedNamespace(t *testing.T) {
	t.Parallel()

	for _, d := range describers() {
		t.Run(d.kind, func(t *testing.T) {
			t.Parallel()
			c, cs := k8stest.NewFakeClient(describeObjects(d.kind)...)
			k8stest.RestrictToNamespace(cs, granted)

			out, err := d.describe(c, context.Background(), granted, "web")
			if err != nil {
				t.Fatalf("Describe%s: %v", d.kind, err)
			}
			if !strings.Contains(out, "readiness probe failed") {
				t.Fatalf("Describe%s lost events readable in the granted namespace:\n%s", d.kind, out)
			}
		})
	}
}
