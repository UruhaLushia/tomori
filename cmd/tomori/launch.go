//go:build !no_gui

package main

import (
	"github.com/spf13/cobra"
)

func newLaunchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "launch <instance>",
		Short: "Launch a configured Minecraft instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			request, err := instanceRequest(true, args[0])
			if err != nil {
				return err
			}
			return runInstance(request)
		},
	}
}
