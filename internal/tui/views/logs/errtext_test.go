package logs

import (
	"context"
	"errors"
	"io"
	"net/url"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Errors read as the classifier words them everywhere else in k4s, minus
// any hint that offers r: the logs view has no r.
func TestErrTextIsClassified(t *testing.T) {
	t.Parallel()

	forbidden := &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure, Code: 403, Reason: metav1.StatusReasonForbidden,
		Message: `pods "web-1" is forbidden: User "dev" cannot get resource "pods/log" in API group "" in the namespace "shop"`,
	}}
	dropped := &url.Error{Op: "Get", URL: "https://10.0.0.1:6443/api/v1/namespaces/shop/pods/web-1/log?follow=true&token=s3cret", Err: io.ErrUnexpectedEOF}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "refused by RBAC", err: forbidden,
			want: "forbidden: get pods/log in shop · kubectl auth can-i get pods --subresource=log -n shop",
		},
		{name: "credentials expired", err: apierrors.NewUnauthorized("token expired"), want: "unauthorized: credentials expired or revoked"},
		{name: "API unavailable", err: apierrors.NewServiceUnavailable("etcd is down"), want: "API unavailable (503)"},
		{name: "API slow", err: context.DeadlineExceeded, want: "API slow: request timed out"},
		{
			name: "a transport error keeps no query", err: dropped,
			want: `Get "https://10.0.0.1:6443/api/v1/namespaces/shop/pods/web-1/log": unexpected EOF`,
		},
		{name: "a panic in k4s", err: internalError{log: "the k4s log"}, want: "internal error (see the k4s log)"},
		{
			name: "the API's own message", err: &url.Error{Op: "Get", URL: dropped.URL, Err: apierrors.NewInternalError(errors.New("boom\x1b]52;c;eA==\x07"))},
			want: "Internal error occurred: boom",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := errText(tc.err); got != tc.want {
				t.Errorf("errText = %q\nwant      %q", got, tc.want)
			}
		})
	}
}
