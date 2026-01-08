package main

import (
	"fmt"
	"time"
)

func parseSince(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	if dur, err := time.ParseDuration(value); err == nil {
		return time.Now().Add(-dur), nil
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"06-01-02 15:04:05.0",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid --since value: %s", value)
}
