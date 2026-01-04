package tmux

import (
	"testing"

	"github.com/dongho-jung/paw/internal/constants"
)

func TestNew(t *testing.T) {
	client := New("test-session")
	if client == nil {
		t.Fatal("New() returned nil")
	}
}

func TestSessionOptsDefaults(t *testing.T) {
	opts := SessionOpts{}

	// Verify zero values
	if opts.Name != "" {
		t.Errorf("SessionOpts.Name should default to empty string")
	}
	if opts.Detached != false {
		t.Errorf("SessionOpts.Detached should default to false")
	}
	if opts.Width != 0 {
		t.Errorf("SessionOpts.Width should default to 0")
	}
	if opts.Height != 0 {
		t.Errorf("SessionOpts.Height should default to 0")
	}
}

func TestWindowOptsDefaults(t *testing.T) {
	opts := WindowOpts{}

	if opts.AfterIndex != 0 {
		t.Errorf("WindowOpts.AfterIndex should default to 0")
	}
	if opts.Detached != false {
		t.Errorf("WindowOpts.Detached should default to false")
	}
}

func TestPopupOptsDefaults(t *testing.T) {
	opts := PopupOpts{}

	if opts.Close != false {
		t.Errorf("PopupOpts.Close should default to false")
	}
	if opts.Env != nil {
		t.Errorf("PopupOpts.Env should default to nil")
	}
}

func TestBindOptsDefaults(t *testing.T) {
	opts := BindOpts{}

	if opts.NoPrefix != false {
		t.Errorf("BindOpts.NoPrefix should default to false")
	}
}

func TestSplitOptsDefaults(t *testing.T) {
	opts := SplitOpts{}

	if opts.Horizontal != false {
		t.Errorf("SplitOpts.Horizontal should default to false (vertical)")
	}
	if opts.Before != false {
		t.Errorf("SplitOpts.Before should default to false")
	}
	if opts.Full != false {
		t.Errorf("SplitOpts.Full should default to false")
	}
}

func TestWindowStruct(t *testing.T) {
	window := Window{
		ID:     "@1",
		Index:  0,
		Name:   "test-window",
		Active: true,
	}

	if window.ID != "@1" {
		t.Errorf("Window.ID = %q, want %q", window.ID, "@1")
	}
	if window.Index != 0 {
		t.Errorf("Window.Index = %d, want %d", window.Index, 0)
	}
	if window.Name != "test-window" {
		t.Errorf("Window.Name = %q, want %q", window.Name, "test-window")
	}
	if window.Active != true {
		t.Errorf("Window.Active = %v, want %v", window.Active, true)
	}
}

func TestTmuxSocketPrefix(t *testing.T) {
	// Verify the socket prefix constant is used correctly
	expectedPrefix := constants.TmuxSocketPrefix
	if expectedPrefix == "" {
		t.Error("TmuxSocketPrefix should not be empty")
	}
}
