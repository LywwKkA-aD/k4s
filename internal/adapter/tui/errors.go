package tui

import (
	"errors"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/adapter/ssh"
)

// ErrorInfo contains user-friendly error information
type ErrorInfo struct {
	Title      string
	Message    string
	Suggestion string
}

// formatUserError converts an error into user-friendly information
func formatUserError(err error) ErrorInfo {
	if err == nil {
		return ErrorInfo{}
	}

	errStr := err.Error()

	// SSH-related errors
	if errors.Is(err, ssh.ErrPassphraseRequired) {
		return ErrorInfo{
			Title:      "Passphrase Required",
			Message:    "The SSH private key is encrypted and requires a passphrase.",
			Suggestion: "Enter the passphrase when prompted, or add your key to ssh-agent:\n  ssh-add ~/.ssh/id_rsa",
		}
	}

	if strings.Contains(errStr, "connection refused") {
		return ErrorInfo{
			Title:      "Connection Refused",
			Message:    "Unable to connect to the remote host.",
			Suggestion: "Check that:\n  • The host address is correct\n  • The SSH service is running on the target\n  • No firewall is blocking the connection",
		}
	}

	if strings.Contains(errStr, "no route to host") {
		return ErrorInfo{
			Title:      "Network Unreachable",
			Message:    "Cannot reach the remote host.",
			Suggestion: "Check that:\n  • The host is online and reachable\n  • Your network connection is working\n  • VPN is connected (if required)",
		}
	}

	if strings.Contains(errStr, "i/o timeout") || strings.Contains(errStr, "connection timed out") {
		return ErrorInfo{
			Title:      "Connection Timeout",
			Message:    "The connection attempt timed out.",
			Suggestion: "Check that:\n  • The host is reachable\n  • The port is correct (default: 22)\n  • No firewall is blocking the connection",
		}
	}

	if strings.Contains(errStr, "unable to authenticate") || strings.Contains(errStr, "handshake failed") {
		return ErrorInfo{
			Title:      "Authentication Failed",
			Message:    "SSH authentication was rejected.",
			Suggestion: "Check that:\n  • The username is correct\n  • The SSH key is authorized on the server\n  • The key file path is correct",
		}
	}

	if strings.Contains(errStr, "no such host") || strings.Contains(errStr, "lookup") {
		return ErrorInfo{
			Title:      "Host Not Found",
			Message:    "The hostname could not be resolved.",
			Suggestion: "Check that:\n  • The hostname is spelled correctly\n  • DNS is working properly\n  • Try using an IP address instead",
		}
	}

	// Kubernetes-related errors
	if strings.Contains(errStr, "kubeconfig") || strings.Contains(errStr, "couldn't get current server") {
		return ErrorInfo{
			Title:      "Kubeconfig Error",
			Message:    "Unable to load or use the kubeconfig file.",
			Suggestion: "Check that:\n  • The kubeconfig file exists and is readable\n  • The file is valid YAML\n  • The cluster context is correct",
		}
	}

	if strings.Contains(errStr, "certificate") || strings.Contains(errStr, "x509") {
		return ErrorInfo{
			Title:      "Certificate Error",
			Message:    "There was a problem with the SSL/TLS certificate.",
			Suggestion: "Check that:\n  • The cluster certificates are valid\n  • The system time is correct\n  • The CA certificate is trusted",
		}
	}

	if strings.Contains(errStr, "Unauthorized") || strings.Contains(errStr, "forbidden") {
		return ErrorInfo{
			Title:      "Access Denied",
			Message:    "You don't have permission to perform this action.",
			Suggestion: "Check that:\n  • Your credentials are valid\n  • You have the required RBAC permissions\n  • The token hasn't expired",
		}
	}

	if strings.Contains(errStr, "not found") {
		return ErrorInfo{
			Title:      "Resource Not Found",
			Message:    "The requested resource does not exist.",
			Suggestion: "The resource may have been deleted or never existed.\nTry refreshing the view with 'r'.",
		}
	}

	if strings.Contains(errStr, "context deadline exceeded") {
		return ErrorInfo{
			Title:      "Request Timeout",
			Message:    "The operation took too long to complete.",
			Suggestion: "The cluster may be under heavy load.\nTry again in a few moments.",
		}
	}

	// Crictl-related errors
	if strings.Contains(errStr, "crictl") {
		if strings.Contains(errStr, "permission denied") {
			return ErrorInfo{
				Title:      "Permission Denied",
				Message:    "crictl requires elevated privileges.",
				Suggestion: "The user needs sudo access to run crictl commands.\nCheck sudoers configuration on the node.",
			}
		}
		if strings.Contains(errStr, "command not found") {
			return ErrorInfo{
				Title:      "crictl Not Found",
				Message:    "crictl is not installed on the remote host.",
				Suggestion: "Install crictl on the node:\n  • For K3s, it should be at /usr/local/bin/crictl",
			}
		}
	}

	// Generic fallback
	return ErrorInfo{
		Title:      "Error",
		Message:    truncateString(errStr, 200),
		Suggestion: "Press 'r' to retry or 'esc' to go back.",
	}
}

// renderErrorBox renders a styled error box
func renderErrorBox(err error, width int) string {
	info := formatUserError(err)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorError)

	messageStyle := lipgloss.NewStyle().
		Foreground(colorText)

	suggestionStyle := lipgloss.NewStyle().
		Foreground(colorMuted).
		Italic(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(colorMuted).
		Bold(true)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2).
		Width(min(width-8, 70))

	var content strings.Builder
	content.WriteString(titleStyle.Render(info.Title))
	content.WriteString("\n\n")
	content.WriteString(messageStyle.Render(info.Message))

	if info.Suggestion != "" {
		content.WriteString("\n\n")
		content.WriteString(labelStyle.Render("Suggestion:"))
		content.WriteString("\n")
		content.WriteString(suggestionStyle.Render(info.Suggestion))
	}

	return boxStyle.Render(content.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
