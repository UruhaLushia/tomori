//go:build !no_gui

package main

import (
	"github.com/spf13/cobra"
)

func newOpenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "open <instance>",
		Short: "Open an instance in the launcher",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			request, err := instanceRequest(false, args[0])
			if err != nil {
				return err
			}
			return runInstance(request)
		},
	}
}
