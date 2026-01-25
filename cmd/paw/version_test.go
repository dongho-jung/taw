package main

import (
	"runtime/debug"
	"testing"
)

func TestApplyBuildInfoUsesModuleVersion(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
	})

	Version = "dev"
	Commit = "unknown"

	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.9.3"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef0123456789"},
		},
	}

	applyBuildInfo(info)

	if Version != "v0.9.3" {
		t.Fatalf("Version = %q, want %q", Version, "v0.9.3")
	}
	if Commit != "abcdef0" {
		t.Fatalf("Commit = %q, want %q", Commit, "abcdef0")
	}
}

func TestApplyBuildInfoSkipsDevelVersion(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
	})

	Version = "dev"
	Commit = "unknown"

	info := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
	}

	applyBuildInfo(info)

	if Version != "dev" {
		t.Fatalf("Version = %q, want %q", Version, "dev")
	}
}

func TestApplyBuildInfoUsesVersionMapForCommit(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalMap := versionCommitMap
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		versionCommitMap = originalMap
	})

	Version = "dev"
	Commit = "unknown"
	versionCommitMap = map[string]string{
		"v9.9.9": "1234567890",
	}

	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v9.9.9"},
	}

	applyBuildInfo(info)

	if Commit != "1234567" {
		t.Fatalf("Commit = %q, want %q", Commit, "1234567")
	}
}

func TestApplyBuildInfoDoesNotOverrideExistingValues(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
	})

	Version = "v1.0.0"
	Commit = "deadbee"

	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v2.0.0"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "cafebabe1234567"},
		},
	}

	applyBuildInfo(info)

	if Version != "v1.0.0" {
		t.Fatalf("Version = %q, want %q", Version, "v1.0.0")
	}
	if Commit != "deadbee" {
		t.Fatalf("Commit = %q, want %q", Commit, "deadbee")
	}
}
