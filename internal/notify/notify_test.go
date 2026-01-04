package notify

import (
	"runtime"
	"testing"
)

func TestSoundTypeConstants(t *testing.T) {
	// Verify sound type constants have expected values
	sounds := map[SoundType]string{
		SoundTaskCreated:   "Glass",
		SoundTaskCompleted: "Hero",
		SoundNeedInput:     "Funk",
		SoundError:         "Basso",
		SoundCancelPending: "Tink",
	}

	for sound, expected := range sounds {
		if string(sound) != expected {
			t.Errorf("SoundType %q = %q, want %q", sound, string(sound), expected)
		}
	}
}

func TestAppNameConstants(t *testing.T) {
	if NotifyAppName != "paw-notify.app" {
		t.Errorf("NotifyAppName = %q, want %q", NotifyAppName, "paw-notify.app")
	}
	if NotifyBinaryName != "paw-notify" {
		t.Errorf("NotifyBinaryName = %q, want %q", NotifyBinaryName, "paw-notify")
	}
}

func TestFindIconPath(t *testing.T) {
	// Just test that it doesn't panic
	path := FindIconPath()
	// Path may or may not exist depending on installation
	_ = path
}

func TestSendNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping non-darwin test on darwin")
	}

	// On non-darwin, Send should return nil without doing anything
	err := Send("Test Title", "Test Message")
	if err != nil {
		t.Errorf("Send() on non-darwin should return nil, got %v", err)
	}
}

func TestSendWithActionsNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping non-darwin test on darwin")
	}

	// On non-darwin, SendWithActions should return -1, nil
	index, err := SendWithActions("Title", "Message", "", []string{"Action1"}, 5)
	if err != nil {
		t.Errorf("SendWithActions() on non-darwin should return nil error, got %v", err)
	}
	if index != -1 {
		t.Errorf("SendWithActions() on non-darwin should return -1, got %d", index)
	}
}

func TestPlaySoundNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping non-darwin test on darwin")
	}

	// On non-darwin, PlaySound should do nothing without panicking
	PlaySound(SoundTaskCreated)
	PlaySound(SoundTaskCompleted)
	PlaySound(SoundNeedInput)
	PlaySound(SoundError)
	PlaySound(SoundCancelPending)
}
