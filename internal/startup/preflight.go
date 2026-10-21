package startup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	apiversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// Preflight defaults.
const (
	defaultPreflightTimeout = 30 * time.Second
	defaultNoticeAfter      = time.Second
	defaultPluginGrace      = 2 * time.Second
)

// ErrAborted reports that the user interrupted the preflight (Ctrl+C).
var ErrAborted = errors.New("aborted")

// errPluginStuck reports a credential plugin that outlived the preflight.
var errPluginStuck = errors.New("credential plugin is still running")

// PreflightConfig tunes Preflight; zero fields take the defaults.
type PreflightConfig struct {
	// Timeout bounds the preflight (default 30s).
	Timeout time.Duration
	// NoticeAfter is how long to wait quietly before saying what k4s is
	// waiting for (default 1s), so a fast login prints nothing.
	NoticeAfter time.Duration
	// Grace is how long a request may outlive Timeout before k4s decides
	// the plugin itself is stuck (default 2s).
	Grace time.Duration
	// Out receives the notice: the real stderr, before the TUI starts.
	Out io.Writer
}

func (c PreflightConfig) withDefaults() PreflightConfig {
	if c.Timeout <= 0 {
		c.Timeout = defaultPreflightTimeout
	}
	if c.NoticeAfter <= 0 {
		c.NoticeAfter = defaultNoticeAfter
	}
	if c.Grace <= 0 {
		c.Grace = defaultPluginGrace
	}
	if c.Out == nil {
		c.Out = io.Discard
	}
	return c
}

// Preflight runs the context's exec or auth-provider credential plugin once
// on the plain terminal, so an SSO device code or MFA prompt can be seen
// and answered before the TUI takes over. Contexts with plain certificates
// or tokens (every stock k3s kubeconfig) skip it and start at once.
//
// A failed or slow preflight lands in PreflightErr and the TUI still
// starts. Cancelling ctx (Ctrl+C) returns ErrAborted. A plugin still
// running after the timeout is an error too: it holds the terminal's
// stdin, which the TUI needs.
func Preflight(ctx context.Context, s Session, cfg PreflightConfig) (Session, error) {
	if s.Client == nil || !s.Client.NeedsLogin() {
		return s, nil
	}
	login, err := s.Client.Interactive()
	if err != nil {
		s.PreflightErr = err
		return s, nil
	}
	info, err := fetchVersion(ctx, login, cfg.withDefaults())
	switch {
	case errors.Is(err, ErrAborted), errors.Is(err, errPluginStuck):
		return s, err
	case err != nil:
		s.PreflightErr = err
		return s, nil
	}
	s.ServerVersion = info
	s.Distro = cluster.Detect(info.GitVersion, s.Client.Context, nil)
	return s, nil
}

type versionResult struct {
	info *apiversion.Info
	err  error
}

// fetchVersion waits for /version in a goroutine: client-go runs the
// plugin without the request's context, so only the wait can be bounded.
func fetchVersion(parent context.Context, c *k8s.Client, cfg PreflightConfig) (*apiversion.Info, error) {
	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()
	done := make(chan versionResult, 1)
	go func() {
		info, err := c.ServerVersion(ctx)
		done <- versionResult{info: info, err: err}
	}()

	host := hostOf(c.RestConfig)
	notice := time.NewTimer(cfg.NoticeAfter)
	defer notice.Stop()
	for {
		select {
		case r := <-done:
			return r.info, r.err
		case <-notice.C:
			_, _ = fmt.Fprintf(cfg.Out, "k4s: connecting to %s…\n", target(c.Context, host))
		case <-ctx.Done():
			if parent.Err() != nil {
				return nil, ErrAborted
			}
			return afterTimeout(done, c.Context, host, cfg)
		}
	}
}

// afterTimeout tells a server that never answered, whose request the
// deadline cancels at once, from a plugin that is still running.
func afterTimeout(done <-chan versionResult, contextName, host string, cfg PreflightConfig) (*apiversion.Info, error) {
	grace := time.NewTimer(cfg.Grace)
	defer grace.Stop()
	select {
	case r := <-done:
		switch {
		case r.err == nil:
			return r.info, nil
		case errors.Is(r.err, context.DeadlineExceeded):
			return nil, fmt.Errorf("preflight: no answer from %s within %s: %w", host, cfg.Timeout, context.DeadlineExceeded)
		}
		return nil, fmt.Errorf("preflight: %w", r.err)
	case <-grace.C:
		return nil, fmt.Errorf("%w for context %q after %s: finish its login, then start k4s again",
			errPluginStuck, contextName, cfg.Timeout)
	}
}

func target(contextName, host string) string {
	if host == "" {
		return contextName
	}
	return contextName + " (" + host + ")"
}

// hostOf is the API server's host:port, without any credentials a URL
// might carry.
func hostOf(cfg *rest.Config) string {
	if cfg == nil || cfg.Host == "" {
		return ""
	}
	raw := cfg.Host
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}
