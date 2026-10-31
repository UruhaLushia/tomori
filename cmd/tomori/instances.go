package main

import (
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/UruhaLushia/tomori/internal/cli"
)

func newInstancesCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "instances",
		Short: "Manage configured Minecraft instances",
		Args:  cobra.NoArgs,
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List configured Minecraft instances",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer cancel()
			asJSON, _ := cmd.Flags().GetBool("json")
			return cli.ListInstances(ctx, asJSON, cmd.OutOrStdout())
		},
	}
	list.Flags().Bool("json", false, "output as JSON")
	command.AddCommand(list)
	return command
}
