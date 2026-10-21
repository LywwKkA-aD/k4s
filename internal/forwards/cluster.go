package forwards

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"net/url"
	"os"

	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// cluster names the cluster a Manager serves: the kubeconfig context, the
// API server address and a fingerprint of the CA the client trusts. Every
// stock k3s kubeconfig calls its context "default" and its server
// https://127.0.0.1:6443, so only the CA tells two of them apart.
type cluster struct {
	contextName string
	server      string
	ca          string
}

func clusterOf(c *k8s.Client) cluster {
	if c == nil {
		return cluster{}
	}
	id := cluster{contextName: c.Context}
	if c.RestConfig != nil {
		id.server = c.RestConfig.Host
		id.ca = caFingerprint(c.RestConfig)
	}
	return id
}

// caFingerprint identifies the CA bundle cfg trusts by the SHA-256 of its
// certificates, so re-encoding the same bundle keeps the fingerprint. It is
// "" when cfg names no CA (system roots, an insecure config) or the file
// cannot be read, in which case the client could not connect either.
func caFingerprint(cfg *rest.Config) string {
	data := cfg.CAData
	if len(data) == 0 && cfg.CAFile != "" {
		b, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return ""
		}
		data = b
	}
	if len(data) == 0 {
		return ""
	}
	sum := sha256.New()
	found := false
	for rest := data; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			sum.Write(block.Bytes)
			found = true
		}
	}
	if !found {
		sum.Write(data)
	}
	return "sha256:" + hex.EncodeToString(sum.Sum(nil))
}

// relation is how a saved forward stands to the cluster a Manager serves.
type relation int

const (
	// sameCluster: saved for this cluster, or the Manager cannot tell.
	sameCluster relation = iota
	// unboundCluster: saved before forwards recorded their server, under
	// this context's name or none; the user confirms before it is revived.
	unboundCluster
	// otherCluster: saved for a different cluster, whatever its context is
	// called.
	otherCluster
)

func (c cluster) relation(f Forward) relation {
	switch {
	case c.contextName == "":
		return sameCluster
	case f.Server == "" && (f.Context == "" || f.Context == c.contextName):
		if c.server == "" {
			return sameCluster
		}
		return unboundCluster
	case f.Context == c.contextName && f.Server == c.server && f.CA == c.ca:
		return sameCluster
	}
	return otherCluster
}

// bind returns f saved for this cluster.
func (c cluster) bind(f Forward) Forward {
	f.Context, f.Server, f.CA = c.contextName, c.server, c.ca
	return f
}

// where is "ctx · host" for the cluster, or the context alone when the
// server is unknown.
func (c cluster) where() string {
	return where(c.contextName, c.server)
}

// UnknownCluster is how a forward saved before k4s recorded servers is
// labelled: the user confirms it belongs to the current cluster by
// reviving it.
const UnknownCluster = "unknown cluster, confirm"

func where(ctx, server string) string {
	ctx = sanitize.Line(ctx)
	if server == "" {
		return ctx
	}
	host := server
	if u, err := url.Parse(server); err == nil && u.Host != "" {
		host = u.Host
	}
	return ctx + " · " + sanitize.Line(host)
}
