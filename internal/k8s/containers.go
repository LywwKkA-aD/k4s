package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Container is one container of a pod as the container picker lists it.
type Container struct {
	Name string
	// Init marks an init container, a native sidecar included. kubectl
	// takes its name with -c like any other container's: an init
	// container that keeps crashing is debugged from its own logs.
	Init bool
	// Sidecar marks a native sidecar, an init container with
	// restartPolicy Always that runs beside the app containers.
	Sidecar bool
}

// Containers lists a pod's containers in picker order: the app
// containers, then the init containers, each in spec order.
type Containers []Container

// Names returns every container's name, in order; nil for none.
func (cs Containers) Names() []string {
	if len(cs) == 0 {
		return nil
	}
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

// InitNames returns the names of the init containers, native sidecars
// included, in order; nil when there are none.
func (cs Containers) InitNames() []string {
	var out []string
	for _, c := range cs {
		if c.Init {
			out = append(out, c.Name)
		}
	}
	return out
}

// ForExec returns the containers a shell can be opened in, in order: the
// app containers and native sidecars, which run as long as the pod does.
// A plain init container has exited before the app starts.
func (cs Containers) ForExec() Containers {
	out := make(Containers, 0, len(cs))
	for _, c := range cs {
		if !c.Init || c.Sidecar {
			out = append(out, c)
		}
	}
	return out
}

// PodContainers returns the containers of the named pod in picker order.
func (c *Client) PodContainers(ctx context.Context, namespace, name string) (Containers, error) {
	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod: %w", err)
	}
	return containersOf(pod.Spec), nil
}

func containersOf(spec corev1.PodSpec) Containers {
	out := make(Containers, 0, len(spec.Containers)+len(spec.InitContainers))
	for _, ct := range spec.Containers {
		out = append(out, Container{Name: ct.Name})
	}
	for _, ct := range spec.InitContainers {
		sidecar := ct.RestartPolicy != nil && *ct.RestartPolicy == corev1.ContainerRestartPolicyAlways
		out = append(out, Container{Name: ct.Name, Init: true, Sidecar: sidecar})
	}
	return out
}
