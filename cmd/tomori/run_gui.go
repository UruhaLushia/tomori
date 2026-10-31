//go:build !no_gui

package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/UruhaLushia/tomori/internal/app"
	"github.com/UruhaLushia/tomori/internal/launchrequest"
	"github.com/UruhaLushia/tomori/locales"
)

func configureRootCommand(root *cobra.Command) {
	root.Args = validateRootArgs
	root.RunE = runRoot
	root.AddCommand(newLaunchCommand(), newOpenCommand())
}

func runRoot(_ *cobra.Command, args []string) error {
	request := launchrequest.Request{}
	if len(args) == 1 {
		var err error
		request, err = launchrequest.ParseURL(args[0])
		if err != nil {
			return err
		}
	}
	return app.Run(request)
}

func validateRootArgs(_ *cobra.Command, args []string) error {
	if len(args) > 1 || len(args) == 1 && !strings.HasPrefix(strings.ToLower(args[0]), "tomori:") {
		return fmt.Errorf("%s", locales.InvalidLaunchArguments())
	}
	return nil
}

func runInstance(request launchrequest.Request) error {
	return app.Run(request)
}
