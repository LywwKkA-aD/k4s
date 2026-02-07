package ssh

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/LywwKkA-aD/k4s/internal/domain"
	"github.com/LywwKkA-aD/k4s/internal/logger"
)

// ErrPassphraseRequired is returned when the private key requires a passphrase
var ErrPassphraseRequired = fmt.Errorf("passphrase required")

// Client wraps SSH connection to a remote host
type Client struct {
	host       domain.SSHHost
	client     *ssh.Client
	passphrase string
}

// NewClient creates a new SSH client for the given host configuration
func NewClient(host domain.SSHHost) *Client {
	return &Client{
		host: host,
	}
}

// SetPassphrase sets the passphrase for the private key
func (c *Client) SetPassphrase(passphrase string) {
	c.passphrase = passphrase
}

// Connect establishes SSH connection to the host
func (c *Client) Connect(ctx context.Context) error {
	var authMethods []ssh.AuthMethod

	// Try ssh-agent first
	if agentAuth := c.trySSHAgent(); agentAuth != nil {
		logger.Debug("SSH agent available, using it for authentication")
		authMethods = append(authMethods, agentAuth)
	}

	// Also try key file if specified
	if c.host.KeyPath != "" {
		keyAuth, err := c.tryKeyFile()
		if err != nil {
			// If no ssh-agent and key file fails, return the error
			if len(authMethods) == 0 {
				return err
			}
			// Otherwise just log and continue with agent
			logger.Debug("Key file auth failed, will use ssh-agent", "err", err)
		} else if keyAuth != nil {
			authMethods = append(authMethods, keyAuth)
		}
	}

	if len(authMethods) == 0 {
		return fmt.Errorf("no authentication methods available")
	}

	// Configure SSH client
	config := &ssh.ClientConfig{
		User:            c.host.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: proper host key verification
		Timeout:         10 * time.Second,
	}

	// Determine port
	port := c.host.Port
	if port == 0 {
		port = 22
	}

	// Connect
	addr := fmt.Sprintf("%s:%d", c.host.Host, port)
	logger.Debug("SSH connecting", "user", c.host.User, "addr", addr)

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}

	c.client = client
	logger.Debug("SSH connected", "addr", addr)
	return nil
}

// trySSHAgent attempts to connect to the SSH agent and get signers
func (c *Client) trySSHAgent() ssh.AuthMethod {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		logger.Debug("SSH_AUTH_SOCK not set, ssh-agent not available")
		return nil
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		logger.Debug("Failed to connect to ssh-agent", "err", err)
		return nil
	}

	agentClient := agent.NewClient(conn)
	signers, err := agentClient.Signers()
	if err != nil {
		logger.Debug("Failed to get signers from ssh-agent", "err", err)
		conn.Close()
		return nil
	}

	if len(signers) == 0 {
		logger.Debug("No keys in ssh-agent")
		conn.Close()
		return nil
	}

	logger.Debug("Found keys in ssh-agent", "count", len(signers))
	return ssh.PublicKeysCallback(agentClient.Signers)
}

// tryKeyFile attempts to use the key file for authentication
func (c *Client) tryKeyFile() (ssh.AuthMethod, error) {
	// Expand ~ in key path
	keyPath := c.host.KeyPath
	if strings.HasPrefix(keyPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
		keyPath = home + keyPath[1:]
	}

	// Read private key
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key %s: %w", keyPath, err)
	}

	// Try to parse private key
	var signer ssh.Signer
	signer, err = ssh.ParsePrivateKey(key)
	if err != nil {
		// Check if passphrase is required
		if strings.Contains(err.Error(), "passphrase") {
			if c.passphrase == "" {
				return nil, ErrPassphraseRequired
			}
			// Try with passphrase
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(c.passphrase))
			if err != nil {
				return nil, fmt.Errorf("parse private key with passphrase: %w", err)
			}
		} else {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
	}

	return ssh.PublicKeys(signer), nil
}

// Close closes the SSH connection
func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// IsConnected returns true if connected
func (c *Client) IsConnected() bool {
	return c.client != nil
}

// Host returns the host configuration
func (c *Client) Host() domain.SSHHost {
	return c.host
}

// Execute runs a command on the remote host and returns the output
func (c *Client) Execute(ctx context.Context, command string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	logger.Debug("SSH executing", "command", command)

	// Run command and capture output
	output, err := session.CombinedOutput(command)
	if err != nil {
		// Include output in error for debugging
		return string(output), fmt.Errorf("execute command: %w", err)
	}

	return string(output), nil
}

// TestConnection tests the SSH connection by running a simple command
func (c *Client) TestConnection(ctx context.Context) error {
	output, err := c.Execute(ctx, "echo ok")
	if err != nil {
		return err
	}
	if strings.TrimSpace(output) != "ok" {
		return fmt.Errorf("unexpected response: %s", output)
	}
	return nil
}
