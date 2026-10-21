package logs

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// joinState is what the view has said about pods joining and pods left
// out.
type joinState struct {
	// told is set once a join has said why: kubectl logs -l does not
	// follow new pods.
	told bool
	// shownOver is the pod count the last "streaming 50 of N" named.
	shownOver int
}

// applyTarget takes a re-resolve: it settles the streams that ended by
// upTo and the pods that wait, starts the pods that joined, and moves the
// footer to the new pod set.
func (m Model) applyTarget(t k8s.LogTarget, upTo uint64) (Model, tea.Cmd) {
	refs := make(map[string]k8s.PodState, len(t.Pods)+len(t.Skipped))
	for _, p := range t.Pods {
		refs[p.Name] = p.State
	}
	for _, s := range t.Skipped {
		refs[s.Name] = s.State
	}
	m, settled := m.judgeEnded(upTo, func(pod string) (k8s.PodState, bool, bool) {
		st, found := refs[pod]
		return st, found, true
	})
	oldHashes := m.target.Hashes
	m.target = t
	m, joined := m.joinPods(t.Pods, oldHashes)
	m.syncPods()
	return m, tea.Batch(settled, joined)
}

// joinPods starts the pods of refs the view does not stream, up to
// maxStreams in all, and says who joined. A pod waiting to start says so
// and joins once it has.
func (m Model) joinPods(refs []k8s.PodRef, oldHashes []string) (Model, tea.Cmd) {
	active := len(m.streams.active())
	var joined []k8s.PodRef
	var reqs []attachReq
	left := 0
	for _, ref := range refs {
		if s, known := m.streams.pods[ref.Name]; known && (s.phase != phaseDone || s.created.Equal(ref.Created)) {
			continue
		}
		if ref.State.Deleting {
			continue
		}
		if active >= maxStreams {
			left++
			continue
		}
		active++
		joined = append(joined, ref)
		m.podColors = withColor(m.podColors, ref.Name)
		s := podStream{container: m.containerFor(ref), created: ref.Created}
		v := judge(end{found: true, state: ref.State, container: s.container, waiting: true})
		var req *attachReq
		m, _, req = m.settlePod(ref.Name, s, v)
		if req != nil {
			reqs = append(reqs, *req)
		}
	}
	cmds := []tea.Cmd{attachCmd(m.sess, reqs)}
	if len(joined) > 0 {
		cmds = append(cmds, statusCmd(status.Info, m.joinText(joined, oldHashes)))
		m.join.told = true
	}
	if total := active + left; left > 0 && total != m.join.shownOver {
		m.join.shownOver = total
		cmds = append(cmds, statusCmd(status.Warn, overText(total)))
	}
	return m, tea.Batch(cmds...)
}

// containerFor is the container a pod streams: the view's, unless the pod
// lacks it (a service can select pods of several workloads), then the
// pod's first.
func (m Model) containerFor(ref k8s.PodRef) string {
	if m.container == "" || len(ref.Containers) == 0 || slices.Contains(ref.Containers, m.container) {
		return m.container
	}
	return ref.Containers[0]
}

// joinText is the status of a join: "+ pod …-k7x2p joined (new
// ReplicaSet 7d9f…)", and on the first one why k4s does it.
func (m Model) joinText(joined []k8s.PodRef, oldHashes []string) string {
	text := "+ " + strconv.Itoa(len(joined)) + " pods joined"
	if len(joined) == 1 {
		text = "+ pod " + shortPod(joined[0].Name) + " joined"
	}
	for _, ref := range joined {
		if ref.Hash != "" && !slices.Contains(oldHashes, ref.Hash) && strings.HasPrefix(ref.Owner, "ReplicaSet/") {
			text += " (new ReplicaSet " + shortHash(ref.Hash) + ")"
			break
		}
	}
	if !m.join.told {
		text += " · " + followNote
	}
	return text
}

// overText says how many pods are left out.
func overText(total int) string {
	return "streaming " + strconv.Itoa(maxStreams) + " of " + strconv.Itoa(total) + " pods · narrow with :ns or a deployment"
}

// shortPod keeps a pod name's last part: "…-k7x2p".
func shortPod(name string) string {
	i := strings.LastIndexByte(name, '-')
	if i <= 0 {
		return name
	}
	return "…" + name[i:]
}

// shortHash keeps the first four characters of a pod-template-hash.
func shortHash(hash string) string {
	if len(hash) <= 4 {
		return hash
	}
	return hash[:4] + "…"
}

// withColor deals pod the next palette colour unless it has one.
func withColor(colors map[string]styles.PodColorID, pod string) map[string]styles.PodColorID {
	if _, ok := colors[pod]; ok {
		return colors
	}
	out := maps.Clone(colors)
	if out == nil {
		out = map[string]styles.PodColorID{}
	}
	out[pod] = podPalette[len(colors)%len(podPalette)]
	return out
}

// syncPods moves the pod list to the streams: a following view lists the
// pods it streams or waits on, so the title and the footer change as pods
// join and leave. Once two pods have streamed, lines carry a prefix.
func (m *Model) syncPods() {
	if !m.following() {
		return
	}
	m.pods = m.streams.active()
	if len(m.pods) > 1 && !m.multi {
		m.multi = true
		m.rebuildRendered()
		m.syncViewport()
	}
}
