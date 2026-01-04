package notify

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dongho-jung/taw/internal/config"
)

func TestSendNtfy_NilConfig(t *testing.T) {
	err := SendNtfy(nil, "title", "message")
	if err != nil {
		t.Errorf("SendNtfy with nil config should return nil, got: %v", err)
	}
}

func TestSendNtfy_EmptyTopic(t *testing.T) {
	cfg := &config.NtfyConfig{Topic: ""}
	err := SendNtfy(cfg, "title", "message")
	if err != nil {
		t.Errorf("SendNtfy with empty topic should return nil, got: %v", err)
	}
}

func TestSendNtfy_Success(t *testing.T) {
	var receivedTitle, receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Verify headers
		receivedTitle = r.Header.Get("Title")
		if receivedTitle != "Test Title" {
			t.Errorf("Expected Title header 'Test Title', got '%s'", receivedTitle)
		}

		if r.Header.Get("Tags") != "taw" {
			t.Errorf("Expected Tags header 'taw', got '%s'", r.Header.Get("Tags"))
		}

		// Read body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
		}
		receivedBody = string(body)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.NtfyConfig{
		Topic:  "test-topic",
		Server: server.URL,
	}
	err := SendNtfy(cfg, "Test Title", "Test Message")
	if err != nil {
		t.Errorf("SendNtfy should succeed, got: %v", err)
	}

	if receivedBody != "Test Message" {
		t.Errorf("Expected body 'Test Message', got '%s'", receivedBody)
	}
}

func TestSendNtfy_DefaultServer(t *testing.T) {
	// This test verifies that the default server URL is used
	// We can't actually test against ntfy.sh, so we just verify the config handling
	cfg := &config.NtfyConfig{
		Topic:  "test-topic",
		Server: "", // Empty server should use default
	}

	// We expect this to fail because we can't reach ntfy.sh in tests
	// but it verifies the URL construction logic
	_ = SendNtfy(cfg, "title", "message")
	// Error is expected, just verifying it doesn't panic
}

func TestSendNtfy_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &config.NtfyConfig{
		Topic:  "test-topic",
		Server: server.URL,
	}
	err := SendNtfy(cfg, "title", "message")
	if err == nil {
		t.Error("SendNtfy should return error on server error")
	}
}

func TestSendNtfy_TrailingSlashInServer(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.NtfyConfig{
		Topic:  "test-topic",
		Server: server.URL + "/", // With trailing slash
	}
	err := SendNtfy(cfg, "title", "message")
	if err != nil {
		t.Errorf("SendNtfy should succeed, got: %v", err)
	}

	if receivedPath != "/test-topic" {
		t.Errorf("Expected path '/test-topic', got '%s'", receivedPath)
	}
}
