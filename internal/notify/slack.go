// Package notify provides notification functionality.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dongho-jung/paw/internal/config"
	"github.com/dongho-jung/paw/internal/logging"
)

const (
	// SlackTimeout is the timeout for Slack webhook requests.
	SlackTimeout = 10 * time.Second
)

// slackMessage represents a Slack webhook message payload.
type slackMessage struct {
	Text        string            `json:"text,omitempty"`
	Attachments []slackAttachment `json:"attachments,omitempty"`
}

// slackAttachment represents a Slack message attachment for rich formatting.
type slackAttachment struct {
	Color  string `json:"color,omitempty"`
	Title  string `json:"title,omitempty"`
	Text   string `json:"text,omitempty"`
	Footer string `json:"footer,omitempty"`
}

// SendSlack sends a notification to Slack via webhook.
// Returns nil if Slack is not configured.
func SendSlack(cfg *config.SlackConfig, title, message string) error {
	if cfg == nil || cfg.Webhook == "" {
		return nil
	}

	logging.Debug("-> SendSlack(title=%q)", title)
	defer logging.Debug("<- SendSlack")

	// Build message with attachment for rich formatting
	payload := slackMessage{
		Attachments: []slackAttachment{
			{
				Color:  "#36a64f", // Green color for PAW notifications
				Title:  title,
				Text:   message,
				Footer: "PAW",
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	client := &http.Client{Timeout: SlackTimeout}
	resp, err := client.Post(cfg.Webhook, "application/json", bytes.NewReader(body))
	if err != nil {
		logging.Warn("SendSlack: failed to send webhook: %v", err)
		return fmt.Errorf("failed to send Slack webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		logging.Warn("SendSlack: webhook returned status %d", resp.StatusCode)
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}

	logging.Debug("SendSlack: notification sent successfully")
	return nil
}
