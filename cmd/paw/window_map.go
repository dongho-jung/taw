package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/dongho-jung/paw/internal/service"
)

var windowMapCmd = &cobra.Command{
	Use:   "window-map",
	Short: "Show window token to task name mappings",
	RunE: func(_ *cobra.Command, _ []string) error {
		application, err := getAppFromCwd()
		if err != nil {
			return err
		}

		mapping, err := service.LoadWindowMap(application.PawDir)
		if err != nil {
			return fmt.Errorf("failed to load window map: %w", err)
		}

		if len(mapping) == 0 {
			fmt.Println("No window mappings found")
			return nil
		}

		keys := make([]string, 0, len(mapping))
		for key := range mapping {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, token := range keys {
			fmt.Printf("%s\t%s\n", token, mapping[token])
		}

		return nil
	},
}
