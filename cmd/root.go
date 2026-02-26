package cmd

import (
	appVer "github.com/librucha/krmgen/version"
	"github.com/spf13/cobra"
)

func NewRootCommand(version string) *cobra.Command {
	var command = &cobra.Command{
		Use:   "krmgen",
		Short: "Kubernetes Resource Model (KRM) generator",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.HelpFunc()(cmd, args)
		},
		Version: version,
	}
	appVer.AppVersion = version
	command.AddCommand(NewGenerateCommand())
	return command
}
