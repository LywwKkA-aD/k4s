package kubectl

import (
	"errors"
	"strconv"
	"strings"
)

// HashLabel is the label a Deployment stamps on each ReplicaSet's pods.
const HashLabel = "pod-template-hash"

// MaxLogRequests is the most pods a follow of several pods streams at
// once: k4s streams no more, and its command asks kubectl for no more.
// kubectl refuses to follow more pods than --max-log-requests allows, so a
// command for more is approximate.
const MaxLogRequests = 50

// ErrEmptySelector is returned for a selector logs command with no selector.
var ErrEmptySelector = errors.New("empty label selector")

// HashSelector narrows a deployment's selector to its ReplicaSets' pods:
// "<selector>,pod-template-hash in (<h1>,<h2>)". Hashes keep their order,
// newest first by convention. No hashes returns selector unchanged.
func HashSelector(selector string, hashes []string) string {
	if len(hashes) == 0 {
		return selector
	}
	clause := HashLabel + " in (" + strings.Join(hashes, ",") + ")"
	if selector == "" {
		return clause
	}
	return selector + "," + clause
}

// LogsSpec describes a multi-pod `kubectl logs -f -l` stream.
type LogsSpec struct {
	Namespace string
	// Selector is the label selector the footer shows and -l receives.
	Selector string
	// Container is passed as -c; empty lets kubectl pick each pod's default.
	Container string
	// Tail is the line count per pod; zero or less means the whole log.
	Tail int64
	// Matched counts the pods Selector matches, finished ones included:
	// kubectl follows them all and refuses to start when they outnumber
	// --max-log-requests.
	Matched int
	// Streamed counts the pods k4s streams, or would: past MaxLogRequests
	// it streams that many.
	Streamed int
}

// SelectorLogs builds
//
//	kubectl logs -f -l '<selector>' --prefix --tail=<n> --max-log-requests=<m> -n <ns> [-c <c>]
//
// --max-log-requests is set whenever more than one pod matches, since
// kubectl's default of 5 makes a larger follow fail, and never above
// MaxLogRequests. The command is Approx when k4s streams fewer pods than
// the selector matches.
func SelectorLogs(s LogsSpec) (Cmd, error) {
	if err := CheckLabel("namespace", s.Namespace); err != nil {
		return Cmd{}, err
	}
	if s.Selector == "" {
		return Cmd{}, ErrEmptySelector
	}
	if err := checkContainer(s.Container); err != nil {
		return Cmd{}, err
	}
	args := []string{"logs", "-f", "-l", s.Selector, "--prefix", tailFlag(s.Tail)}
	streamed := min(s.Streamed, MaxLogRequests)
	if requests := min(max(s.Matched, streamed), MaxLogRequests); requests > 1 {
		args = append(args, "--max-log-requests="+strconv.Itoa(requests))
	}
	args = append(args, "-n", s.Namespace)
	args = withContainer(args, s.Container)
	return Cmd{Args: args, Approx: streamed < s.Matched}, nil
}

// PodLogs builds `kubectl logs -f <pod> --tail=<n> -n <ns> [-c <c>]`.
func PodLogs(namespace, pod, container string, tail int64) (Cmd, error) {
	if err := CheckLabel("namespace", namespace); err != nil {
		return Cmd{}, err
	}
	if err := CheckSubdomain("pod", pod); err != nil {
		return Cmd{}, err
	}
	if err := checkContainer(container); err != nil {
		return Cmd{}, err
	}
	args := withContainer([]string{"logs", "-f", pod, tailFlag(tail), "-n", namespace}, container)
	return Cmd{Args: args}, nil
}

// Exec builds `kubectl exec -it -n <ns> [-c <c>] <pod> -- <command...>`.
func Exec(namespace, pod, container string, command ...string) (Cmd, error) {
	if err := CheckLabel("namespace", namespace); err != nil {
		return Cmd{}, err
	}
	if err := CheckSubdomain("pod", pod); err != nil {
		return Cmd{}, err
	}
	if err := checkContainer(container); err != nil {
		return Cmd{}, err
	}
	if len(command) == 0 {
		return Cmd{}, ErrNoCommand
	}
	args := withContainer([]string{"exec", "-it", "-n", namespace}, container)
	args = append(args, pod, "--")
	return Cmd{Args: append(args, command...)}, nil
}

// tailFlag spells out --tail even for the whole log: with -l kubectl
// defaults to the last 10 lines, not to everything.
func tailFlag(n int64) string {
	if n <= 0 {
		return "--tail=-1"
	}
	return "--tail=" + strconv.FormatInt(n, 10)
}

func checkContainer(name string) error {
	if name == "" {
		return nil
	}
	return CheckLabel("container", name)
}

func withContainer(args []string, container string) []string {
	if container == "" {
		return args
	}
	return append(args, "-c", container)
}
