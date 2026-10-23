package kubectl

import (
	"errors"
	"strings"
)

// ErrSessionFlag is wrapped by every FlagError.
var ErrSessionFlag = errors.New("session flag in args")

// FlagError reports a word in Cmd.Args that sets a flag k4s never puts
// there: one that picks the cluster, the credentials or a file kubectl
// writes. Words in Args can come from the cluster, and such a word, run or
// pasted, would override the flags Decorate adds.
type FlagError struct {
	// Flag is the flag as k4s names it ("--server", "-s"), never the word
	// that set it.
	Flag string
}

func (e *FlagError) Error() string { return "the command sets " + e.Flag }

// Unwrap makes errors.Is(err, ErrSessionFlag) hold.
func (e *FlagError) Unwrap() error { return ErrSessionFlag }

// sessionFlags are the long flags, without their dashes, that only the
// user's own kubectl setup may set. --kubeconfig and --context reach a
// command through Decorate, which keeps them out of Args.
var sessionFlags = map[string]bool{
	"kubeconfig":               true,
	"context":                  true,
	"cluster":                  true,
	"user":                     true,
	"server":                   true,
	"token":                    true,
	"as":                       true,
	"as-group":                 true,
	"as-uid":                   true,
	"username":                 true,
	"password":                 true,
	"certificate-authority":    true,
	"client-certificate":       true,
	"client-key":               true,
	"insecure-skip-tls-verify": true,
	"tls-server-name":          true,
	"log-file":                 true,
	"log-dir":                  true,
	"profile-output":           true,
	"cache-dir":                true,
	"kuberc":                   true,
}

// CheckFlags returns a *FlagError for the first word of c.Args before "--"
// that sets a session flag, in any form kubectl parses: --server x,
// --server=x, -s x, -s=x, -sx, or -s inside a cluster of shorthands such
// as -As=x. Words after "--" belong to a remote command and are left
// alone. The check is a backstop: builders refuse names from the cluster
// that kubectl would read as a flag.
func CheckFlags(c Cmd) error {
	for _, w := range c.Args {
		if w == "--" {
			return nil
		}
		if flag := sessionFlag(w); flag != "" {
			return &FlagError{Flag: flag}
		}
	}
	return nil
}

// sessionFlag names the session flag word w sets, "" for none. Any s in a
// shorthand cluster counts: which shorthands before it take a value is
// up to the subcommand, and k4s writes no cluster with an s.
func sessionFlag(w string) string {
	if name, ok := strings.CutPrefix(w, "--"); ok {
		name, _, _ = strings.Cut(name, "=")
		if sessionFlags[name] {
			return "--" + name
		}
		return ""
	}
	shorts, ok := strings.CutPrefix(w, "-")
	if !ok {
		return ""
	}
	shorts, _, _ = strings.Cut(shorts, "=")
	if strings.ContainsRune(shorts, 's') {
		return "-s"
	}
	return ""
}
