package cmd

import (
	"github.com/librucha/krmgen/version"
	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	var command = &cobra.Command{
		Use:   "krmgen",
		Short: "Kubernetes Resource Model (KRM) generator",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
		Version: version.AppVersion,
	}
	command.AddCommand(NewGenerateCommand())
	return command
}
