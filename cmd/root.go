package cmd

import (
	"github.com/spf13/cobra"
)

var appVersion string

func NewRootCommand() *cobra.Command {
	var command = &cobra.Command{
		Use:   "krmgen",
		Short: "Kubernetes Resource Model (KRM) generator",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
		Version: appVersion,
	}
	command.AddCommand(NewGenerateCommand())
	return command
}

func SetAppVersion(version string) {
	appVersion = version
}
