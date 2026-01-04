package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dongho-jung/taw/internal/config"
)

func TestSendSlack_NilConfig(t *testing.T) {
	err := SendSlack(nil, "title", "message")
	if err != nil {
		t.Errorf("SendSlack with nil config should return nil, got: %v", err)
	}
}

func TestSendSlack_EmptyWebhook(t *testing.T) {
	cfg := &config.SlackConfig{Webhook: ""}
	err := SendSlack(cfg, "title", "message")
	if err != nil {
		t.Errorf("SendSlack with empty webhook should return nil, got: %v", err)
	}
}

func TestSendSlack_Success(t *testing.T) {
	// Create a test server that simulates Slack webhook
	var receivedPayload slackMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Verify content type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Read and parse body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
		}

		if err := json.Unmarshal(body, &receivedPayload); err != nil {
			t.Errorf("Failed to unmarshal request body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.SlackConfig{Webhook: server.URL}
	err := SendSlack(cfg, "Test Title", "Test Message")
	if err != nil {
		t.Errorf("SendSlack should succeed, got: %v", err)
	}

	// Verify payload
	if len(receivedPayload.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(receivedPayload.Attachments))
	}

	if receivedPayload.Attachments[0].Title != "Test Title" {
		t.Errorf("Expected title 'Test Title', got '%s'", receivedPayload.Attachments[0].Title)
	}

	if receivedPayload.Attachments[0].Text != "Test Message" {
		t.Errorf("Expected text 'Test Message', got '%s'", receivedPayload.Attachments[0].Text)
	}

	if receivedPayload.Attachments[0].Footer != "TAW" {
		t.Errorf("Expected footer 'TAW', got '%s'", receivedPayload.Attachments[0].Footer)
	}
}

func TestSendSlack_ServerError(t *testing.T) {
	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &config.SlackConfig{Webhook: server.URL}
	err := SendSlack(cfg, "title", "message")
	if err == nil {
		t.Error("SendSlack should return error on server error")
	}
}

func TestSendSlack_InvalidURL(t *testing.T) {
	cfg := &config.SlackConfig{Webhook: "http://invalid.localhost:99999/webhook"}
	err := SendSlack(cfg, "title", "message")
	if err == nil {
		t.Error("SendSlack should return error on invalid URL")
	}
}
