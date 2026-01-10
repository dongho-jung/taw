// Package notify provides notification functionality.
package notify

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/logging"
)

const (
	// NtfyTimeout is the timeout for ntfy requests.
	NtfyTimeout = 10 * time.Second
	// NtfyDefaultServer is the default ntfy server URL.
	NtfyDefaultServer = "https://ntfy.sh"
)

// SendNtfy sends a notification to ntfy.sh (or a self-hosted ntfy server).
// Returns nil if ntfy is not configured.
func SendNtfy(cfg *config.NtfyConfig, title, message string) error {
	if cfg == nil || cfg.Topic == "" {
		return nil
	}

	logging.Debug("-> SendNtfy(title=%q, topic=%q)", title, cfg.Topic)
	defer logging.Debug("<- SendNtfy")

	// Determine server URL
	server := cfg.Server
	if server == "" {
		server = NtfyDefaultServer
	}

	// Build URL: server/topic
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(server, "/"), cfg.Topic)

	// Create request with title and message
	req, err := http.NewRequest("POST", url, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("failed to create ntfy request: %w", err)
	}

	// Set headers for ntfy
	req.Header.Set("Title", title)
	req.Header.Set("Priority", "default")
	req.Header.Set("Tags", "paw")

	client := &http.Client{Timeout: NtfyTimeout}
	resp, err := client.Do(req)
	if err != nil {
		logging.Warn("SendNtfy: failed to send notification: %v", err)
		return fmt.Errorf("failed to send ntfy notification: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		logging.Warn("SendNtfy: server returned status %d", resp.StatusCode)
		return fmt.Errorf("ntfy server returned status %d", resp.StatusCode)
	}

	logging.Debug("SendNtfy: notification sent successfully to %s", url)
	return nil
}
