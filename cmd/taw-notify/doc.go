//go:build !darwin

// Package main is a macOS-only notification helper.
// This package only builds on macOS (darwin).
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "taw-notify is only supported on macOS")
	os.Exit(1)
}
