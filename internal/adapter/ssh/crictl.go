package ssh

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/LywwKkA-aD/k4s/internal/domain"
	"github.com/LywwKkA-aD/k4s/internal/logger"
)

// CrictlContainer represents a container from crictl ps output
type CrictlContainer struct {
	ContainerID      string // Full container ID
	ContainerIDShort string // Truncated for display
	Image            string
	Created          string
	CreatedAt        int64 // Unix nano timestamp
	State            string
	Name             string
	PodID            string
	PodName          string
	Namespace        string
}

// crictlPsJSON represents the JSON output from crictl ps
type crictlPsJSON struct {
	Containers []crictlContainerJSON `json:"containers"`
}

type crictlContainerJSON struct {
	ID          string            `json:"id"`
	PodSandboxID string           `json:"podSandboxId"`
	Metadata    containerMetadata `json:"metadata"`
	Image       imageSpec         `json:"image"`
	ImageRef    string            `json:"imageRef"`
	State       string            `json:"state"`
	CreatedAt   string            `json:"createdAt"`
	Labels      map[string]string `json:"labels"`
}

type containerMetadata struct {
	Name    string `json:"name"`
	Attempt uint32 `json:"attempt"`
}

type imageSpec struct {
	Image string `json:"image"`
}

// CrictlPod represents a pod from crictl pods output
type CrictlPod struct {
	PodID     string
	Created   string
	State     string
	Name      string
	Namespace string
}

// CrictlImage represents an image from crictl images output
type CrictlImage struct {
	ImageID string
	Tags    string
	Size    string
}

// ListContainers runs crictl ps and returns container list
func (c *Client) ListContainers(ctx context.Context) ([]CrictlContainer, error) {
	// Use sudo for crictl with JSON output for reliable parsing
	output, err := c.Execute(ctx, "sudo crictl ps -a -o json")
	if err != nil {
		return nil, fmt.Errorf("crictl ps: %w", err)
	}

	return parseCrictlPsJSON(output)
}

// ListPods runs crictl pods and returns pod list
func (c *Client) ListPods(ctx context.Context) ([]CrictlPod, error) {
	output, err := c.Execute(ctx, "sudo crictl pods")
	if err != nil {
		return nil, fmt.Errorf("crictl pods: %w", err)
	}

	return parseCrictlPods(output), nil
}

// ListImages runs crictl images and returns image list
func (c *Client) ListImages(ctx context.Context) ([]CrictlImage, error) {
	output, err := c.Execute(ctx, "sudo crictl images")
	if err != nil {
		return nil, fmt.Errorf("crictl images: %w", err)
	}

	return parseCrictlImages(output), nil
}

// InspectContainer runs crictl inspect on a container
func (c *Client) InspectContainer(ctx context.Context, containerID string) (string, error) {
	output, err := c.Execute(ctx, fmt.Sprintf("sudo crictl inspect %s", containerID))
	if err != nil {
		return "", fmt.Errorf("crictl inspect: %w", err)
	}
	return output, nil
}

// CrictlLogOptions represents options for crictl logs
type CrictlLogOptions struct {
	TailLines  int64
	Follow     bool
	Timestamps bool
}

// ContainerLogs gets logs for a container via crictl
func (c *Client) ContainerLogs(ctx context.Context, containerID string, opts CrictlLogOptions) (string, error) {
	var args []string
	args = append(args, "sudo", "crictl", "logs")

	if opts.TailLines > 0 {
		args = append(args, fmt.Sprintf("--tail=%d", opts.TailLines))
	}
	if opts.Timestamps {
		args = append(args, "--timestamps")
	}
	args = append(args, containerID)

	cmd := strings.Join(args, " ")
	output, err := c.Execute(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("crictl logs: %w", err)
	}
	return output, nil
}

// StreamContainerLogs streams logs for a container via crictl
func (c *Client) StreamContainerLogs(ctx context.Context, containerID string, opts CrictlLogOptions, lineChan chan<- string) error {
	if c.client == nil {
		return fmt.Errorf("not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	// Build command
	var args []string
	args = append(args, "sudo", "crictl", "logs", "-f")

	if opts.TailLines > 0 {
		args = append(args, fmt.Sprintf("--tail=%d", opts.TailLines))
	} else {
		// When streaming without tail, use --since to only get recent logs
		args = append(args, "--since=1s")
	}
	if opts.Timestamps {
		args = append(args, "--timestamps")
	}
	args = append(args, containerID)

	cmd := strings.Join(args, " ")
	logger.Debug("Streaming crictl logs", "command", cmd)

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return fmt.Errorf("get stdout pipe: %w", err)
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		return fmt.Errorf("get stderr pipe: %w", err)
	}

	if err := session.Start(cmd); err != nil {
		session.Close()
		return fmt.Errorf("start command: %w", err)
	}

	// Read from both stdout and stderr
	go func() {
		defer session.Close()

		// Create scanner for stdout
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case lineChan <- scanner.Text():
			}
		}
	}()

	// Also read stderr (crictl sometimes outputs to stderr)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case lineChan <- scanner.Text():
			}
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	session.Signal(ssh.SIGTERM)
	return ctx.Err()
}

// GetNodeInfo gets basic node information
func (c *Client) GetNodeInfo(ctx context.Context) (*domain.NodeInfo, error) {
	// Get hostname
	hostname, err := c.Execute(ctx, "hostname")
	if err != nil {
		return nil, fmt.Errorf("get hostname: %w", err)
	}

	// Get OS info
	osInfo, _ := c.Execute(ctx, "cat /etc/os-release | grep PRETTY_NAME | cut -d'\"' -f2")

	// Get kernel version
	kernel, _ := c.Execute(ctx, "uname -r")

	// Get uptime
	uptime, _ := c.Execute(ctx, "uptime -p")

	// Get memory info
	memInfo, _ := c.Execute(ctx, "free -h | grep Mem | awk '{print $3 \"/\" $2}'")

	// Get disk usage
	diskInfo, _ := c.Execute(ctx, "df -h / | tail -1 | awk '{print $3 \"/\" $2 \" (\" $5 \")\"}'")

	// Get CPU load
	loadAvg, _ := c.Execute(ctx, "cat /proc/loadavg | awk '{print $1 \" \" $2 \" \" $3}'")

	return &domain.NodeInfo{
		Hostname:  strings.TrimSpace(hostname),
		OS:        strings.TrimSpace(osInfo),
		Kernel:    strings.TrimSpace(kernel),
		Uptime:    strings.TrimSpace(uptime),
		Memory:    strings.TrimSpace(memInfo),
		Disk:      strings.TrimSpace(diskInfo),
		LoadAvg:   strings.TrimSpace(loadAvg),
	}, nil
}

// parseCrictlPsJSON parses crictl ps JSON output
func parseCrictlPsJSON(output string) ([]CrictlContainer, error) {
	var psOutput crictlPsJSON
	if err := json.Unmarshal([]byte(output), &psOutput); err != nil {
		return nil, fmt.Errorf("parse crictl ps JSON: %w", err)
	}

	var containers []CrictlContainer
	for _, c := range psOutput.Containers {
		// Parse createdAt timestamp (nanoseconds since epoch)
		createdAt, _ := strconv.ParseInt(c.CreatedAt, 10, 64)
		createdTime := time.Unix(0, createdAt)
		created := formatAge(createdTime)

		// Extract image name (strip registry prefix for display)
		imageName := c.Image.Image
		if parts := strings.Split(imageName, "/"); len(parts) > 0 {
			imageName = parts[len(parts)-1]
		}
		// Truncate long image names
		if len(imageName) > 30 {
			imageName = imageName[:27] + "..."
		}

		// Extract pod name and namespace from labels
		podName := c.Labels["io.kubernetes.pod.name"]
		namespace := c.Labels["io.kubernetes.pod.namespace"]

		containers = append(containers, CrictlContainer{
			ContainerID:      c.ID,
			ContainerIDShort: truncateID(c.ID),
			Image:            imageName,
			Created:          created,
			CreatedAt:        createdAt,
			State:            strings.ToLower(c.State),
			Name:             c.Metadata.Name,
			PodID:            truncateID(c.PodSandboxID),
			PodName:          podName,
			Namespace:        namespace,
		})
	}
	return containers, nil
}

// truncateID truncates container/pod IDs for display
func truncateID(id string) string {
	if len(id) > 13 {
		return id[:13]
	}
	return id
}

// formatAge formats a time as a human-readable age string
func formatAge(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	}
	if duration < 24*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	days := int(duration.Hours() / 24)
	return fmt.Sprintf("%dd", days)
}

// parseCrictlPods parses crictl pods output
func parseCrictlPods(output string) []CrictlPod {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 1 {
		return nil
	}

	var pods []CrictlPod
	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			pods = append(pods, CrictlPod{
				PodID:     fields[0],
				Created:   fields[1],
				State:     fields[2],
				Name:      fields[3],
				Namespace: fields[4],
			})
		}
	}
	return pods
}

// parseCrictlImages parses crictl images output
func parseCrictlImages(output string) []CrictlImage {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 1 {
		return nil
	}

	var images []CrictlImage
	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			images = append(images, CrictlImage{
				ImageID: fields[0],
				Tags:    fields[1],
				Size:    fields[len(fields)-1],
			})
		}
	}
	return images
}
