package cmd

import (
	"fmt"
	"github.com/librucha/krmgen/internal/config"
	"github.com/spf13/cobra"
	"log"
	"os"
	"path/filepath"
)

func NewGenerateCommand() *cobra.Command {
	command := &cobra.Command{
		Use:     "generate <path>",
		Short:   "Generate KRM by declared config",
		Aliases: []string{"g"},
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("<path> argument required to generate KRM")
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			workDir, err := filepath.Abs(args[0])
			if err != nil {
				log.Fatal(err)
			}
			if err := processWorkDir(workDir); err != nil {
				log.Fatal(err)
			}
		},
	}
	return command
}

func processWorkDir(workDir string) error {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		filePath := workDir + "/" + entry.Name()
		if !entry.IsDir() && config.IsConfigFile(filePath) {
			configObject, err := config.ParseConfig(filePath)
			if err != nil {
				return err
			}
			resources, err := config.ProcessConfig(configObject, workDir)
			if err != nil {
				return err
			}
			fmt.Println(resources)
		}
	}
	return nil
}
