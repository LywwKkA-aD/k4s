package k8s

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// writeContainers writes a "Containers:" (or "Init Containers:") section.
// pod is nil for a pod template, which has no Container ID, Image ID or
// state; space indents the section inside one.
func writeContainers(w describeWriter, label, space string, pod *corev1.Pod, containers []corev1.Container, statuses []corev1.ContainerStatus) {
	none := ""
	if len(containers) == 0 {
		none = " <none>"
	}
	w.write(0, "%s%s:%s\n", space, label, none)
	for i := range containers {
		c := &containers[i]
		st, ok := statusOf(statuses, c.Name)
		writeContainerInfo(w, space, c, st, ok)
		writeCommand(w, c)
		if ok {
			writeState(w, "State", st.State)
			if st.LastTerminationState.Terminated != nil {
				writeState(w, "Last State", st.LastTerminationState)
			}
			w.write(2, "Ready:\t%s\n", boolText(st.Ready))
			w.write(2, "Restart Count:\t%d\n", st.RestartCount)
		}
		writeResources(w, c.Resources)
		writeProbes(w, c)
		writeEnvFrom(w, c.EnvFrom)
		writeEnv(w, pod, c)
		writeMounts(w, c.VolumeMounts)
	}
}

func statusOf(statuses []corev1.ContainerStatus, name string) (*corev1.ContainerStatus, bool) {
	i := slices.IndexFunc(statuses, func(s corev1.ContainerStatus) bool { return s.Name == name })
	if i < 0 {
		return nil, false
	}
	return &statuses[i], true
}

func writeContainerInfo(w describeWriter, space string, c *corev1.Container, st *corev1.ContainerStatus, ok bool) {
	nameIndent := ""
	if space != "" {
		nameIndent = " "
	}
	w.write(1, "%s%s:\n", nameIndent, c.Name)
	if ok {
		w.write(2, "Container ID:\t%s\n", st.ContainerID)
	}
	w.write(2, "Image:\t%s\n", c.Image)
	if ok {
		w.write(2, "Image ID:\t%s\n", st.ImageID)
	}
	ports, hostPorts := containerPorts(c.Ports)
	if len(c.Ports) > 1 {
		w.write(2, "Ports:\t%s\n", ports)
		w.write(2, "Host Ports:\t%s\n", hostPorts)
	} else {
		w.write(2, "Port:\t%s\n", orNone(ports))
		w.write(2, "Host Port:\t%s\n", orNone(hostPorts))
	}
}

// containerPorts lists "80/TCP, 443/TCP" and the host ports the same way.
// The API server fills in TCP; an object built without it says so too.
func containerPorts(ports []corev1.ContainerPort) (string, string) {
	cp := make([]string, 0, len(ports))
	hp := make([]string, 0, len(ports))
	for _, p := range ports {
		proto := p.Protocol
		if proto == "" {
			proto = corev1.ProtocolTCP
		}
		cp = append(cp, fmt.Sprintf("%d/%s", p.ContainerPort, proto))
		hp = append(hp, fmt.Sprintf("%d/%s", p.HostPort, proto))
	}
	return strings.Join(cp, ", "), strings.Join(hp, ", ")
}

func writeCommand(w describeWriter, c *corev1.Container) {
	writeWords(w, "Command", c.Command)
	writeWords(w, "Args", c.Args)
}

// writeWords writes each word on its own line, and each line of a
// multi-line word, as kubectl does for a command and its args.
func writeWords(w describeWriter, title string, words []string) {
	if len(words) == 0 {
		return
	}
	w.write(2, "%s:\n", title)
	for _, word := range words {
		for line := range strings.SplitSeq(word, "\n") {
			w.write(3, "%s\n", line)
		}
	}
}

// writeState writes a container state with its reason, exit code and
// times: "State:" for the current one, "Last State:" for the last
// termination.
func writeState(w describeWriter, title string, s corev1.ContainerState) {
	switch {
	case s.Running != nil:
		w.write(2, "%s:\tRunning\n", title)
		w.write(3, "Started:\t%s\n", stamp(s.Running.StartedAt))
	case s.Terminated != nil:
		t := s.Terminated
		w.write(2, "%s:\tTerminated\n", title)
		if t.Reason != "" {
			w.write(3, "Reason:\t%s\n", t.Reason)
		}
		if t.Message != "" {
			w.write(3, "Message:\t%s\n", t.Message)
		}
		w.write(3, "Exit Code:\t%d\n", t.ExitCode)
		if t.Signal > 0 {
			w.write(3, "Signal:\t%d\n", t.Signal)
		}
		w.write(3, "Started:\t%s\n", stamp(t.StartedAt))
		w.write(3, "Finished:\t%s\n", stamp(t.FinishedAt))
	default:
		w.write(2, "%s:\tWaiting\n", title)
		if s.Waiting != nil && s.Waiting.Reason != "" {
			w.write(3, "Reason:\t%s\n", s.Waiting.Reason)
		}
	}
}

// writeResources writes limits, then requests, each in resource-name
// order: a map would print them in a different order every time.
func writeResources(w describeWriter, r corev1.ResourceRequirements) {
	writeResourceList(w, "Limits", r.Limits)
	writeResourceList(w, "Requests", r.Requests)
}

func writeResourceList(w describeWriter, title string, list corev1.ResourceList) {
	if len(list) == 0 {
		return
	}
	names := make([]string, 0, len(list))
	for name := range list {
		names = append(names, string(name))
	}
	slices.Sort(names)
	w.write(2, "%s:\n", title)
	for _, name := range names {
		q := list[corev1.ResourceName(name)]
		w.write(3, "%s:\t%s\n", name, q.String())
	}
}

// writeMounts writes the volume mounts in mount-path order, each with
// ro or rw and its sub-path.
func writeMounts(w describeWriter, mounts []corev1.VolumeMount) {
	if len(mounts) == 0 {
		w.write(2, "Mounts:\t<none>\n")
		return
	}
	w.write(2, "Mounts:\n")
	sorted := slices.Clone(mounts)
	slices.SortStableFunc(sorted, func(a, b corev1.VolumeMount) int { return strings.Compare(a.MountPath, b.MountPath) })
	for _, m := range sorted {
		flags := []string{"rw"}
		if m.ReadOnly {
			flags[0] = "ro"
		}
		if m.SubPath != "" {
			flags = append(flags, "path="+strconv.Quote(m.SubPath))
		}
		w.write(3, "%s from %s (%s)\n", m.MountPath, m.Name, strings.Join(flags, ","))
	}
}

func boolText(b bool) string {
	if b {
		return "True"
	}
	return "False"
}
