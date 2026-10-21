package cluster

import (
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// Name is how the header names d: the distribution, or "k8s" for one
// Detect does not tell apart. The zero Distro has no name.
func (d Distro) Name() string {
	if d == Other {
		return "k8s"
	}
	return string(d)
}

var releasePattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+`)

// Release is the Kubernetes release a /version gitVersion names, without
// the vendor's build suffix: v1.36.5+k3s1 and v1.30.4-eks-a737599 are
// v1.36.5 and v1.30.4. It is "" when gitVersion does not start with one.
func Release(gitVersion string) string {
	return releasePattern.FindString(gitVersion)
}

// Host is the API server's host as the header shows it: no scheme, path or
// credentials, and no port when it is 443 or 6443 (the k3s default). It is
// "" when server cannot be parsed.
func Host(server string) string {
	if server == "" {
		return ""
	}
	if !strings.Contains(server, "://") {
		server = "https://" + server
	}
	u, err := url.Parse(server)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	host := u.Hostname()
	if port := u.Port(); port != "" && port != "443" && port != "6443" {
		host = net.JoinHostPort(host, port)
	}
	return sanitize.Line(host)
}

// ambiguousName is the context every stock k3s kubeconfig defines.
const ambiguousName = "default"

// Ambiguous reports whether the context name alone does not say which
// cluster k4s talks to: it is "default", or more than one of the
// kubeconfig files defines it (the first one wins when they are merged).
// Files that are missing or cannot be read are skipped, as kubectl does.
func Ambiguous(name string, files []string) bool {
	if name == "" {
		return false
	}
	if name == ambiguousName {
		return true
	}
	defining := 0
	var seen []string
	for _, f := range files {
		if slices.Contains(seen, f) {
			continue
		}
		seen = append(seen, f)
		cfg, err := clientcmd.LoadFromFile(f)
		if err != nil {
			continue
		}
		if _, ok := cfg.Contexts[name]; ok {
			defining++
		}
	}
	return defining > 1
}
