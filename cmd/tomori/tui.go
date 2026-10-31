//go:build !no_tui

package main

import (
	"github.com/spf13/cobra"

	"github.com/UruhaLushia/tomori/internal/tui"
)

func addTUICommand(root *cobra.Command) {
	root.AddCommand(&cobra.Command{
		Use:   "tui",
		Short: "Terminal interface (not implemented yet)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return tui.Run()
		},
	})
}
