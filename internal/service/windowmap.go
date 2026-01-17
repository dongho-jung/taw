// Package service provides business logic services for PAW.
package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dongho-jung/paw/internal/constants"
)

// UpdateWindowMap records the mapping between a window token and full task name.
func UpdateWindowMap(pawDir, taskName string) (string, error) {
	token := constants.TruncateForWindowName(taskName)
	mapPath := filepath.Join(pawDir, constants.WindowMapFileName)
	if err := os.MkdirAll(filepath.Dir(mapPath), 0755); err != nil {
		return token, fmt.Errorf("failed to create window map directory: %w", err)
	}

	mapping := map[string]string{}
	if data, err := os.ReadFile(mapPath); err == nil {
		_ = json.Unmarshal(data, &mapping)
	}

	mapping[token] = taskName
	data, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return token, fmt.Errorf("failed to marshal window map: %w", err)
	}
	if err := os.WriteFile(mapPath, data, 0644); err != nil {
		return token, fmt.Errorf("failed to write window map: %w", err)
	}

	return token, nil
}

// LoadWindowMap reads the window token map from disk.
func LoadWindowMap(pawDir string) (map[string]string, error) {
	mapPath := filepath.Join(pawDir, constants.WindowMapFileName)
	data, err := os.ReadFile(mapPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}

	mapping := map[string]string{}
	if err := json.Unmarshal(data, &mapping); err != nil {
		return nil, err
	}
	return mapping, nil
}
