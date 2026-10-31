package main

import (
	"github.com/spf13/cobra"
)

const version = "0.1.0"

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "tomori",
		Short:         "Minecraft launcher",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newInstancesCommand())
	addTUICommand(root)
	configureRootCommand(root)
	return root
}
