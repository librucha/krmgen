package cmd

import (
	"fmt"
	"github.com/librucha/krmgen/internal/config"
	"github.com/librucha/krmgen/internal/template"
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
			srcDir, err := filepath.Abs(args[0])
			if err != nil {
				log.Fatal(err)
			}
			workDir := copySrcDir(srcDir)
			processWorkDir(workDir)
			defer func(path string) {
				_ = os.RemoveAll(path)
			}(workDir)
		},
	}
	return command
}

func copySrcDir(srcDir string) string {

	workDir, err := os.MkdirTemp(os.TempDir(), "krmgen")
	if err != nil {
		log.Fatalf("creating working dir in %s failed error: %s", os.TempDir(), err)
	}

	copyDir(srcDir, workDir)

	return workDir
}

func copyDir(srcDir string, dstDir string) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		log.Fatalf("reading source directory %s failed error: %s", srcDir, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		if entry.IsDir() {
			err = os.MkdirAll(dstPath, 0750)
			if err != nil {
				log.Fatalf("crating directory %s failed error: %s", dstPath, err)
			}
			copyDir(filepath.Join(srcDir, entry.Name()), dstPath)
		} else {
			fileContent, err := os.ReadFile(srcPath)
			if err != nil {
				log.Fatalf("reading file %s failed error: %s", srcPath, err)
			}
			// evaluate templates
			evaluated, err := template.EvalGoTemplates(string(fileContent))
			if err != nil {
				log.Fatalf("template evaluation of file %s failed error: %s", srcPath, err)
			}
			err = os.WriteFile(dstPath, []byte(evaluated), os.ModePerm)
			if err != nil {
				log.Fatalf("writing evaluated file %s failed error: %s", srcPath, err)
			}
		}

	}
}

func processWorkDir(workDir string) {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		log.Fatalf("reading work directory %s failed error: %s", workDir, err)
	}

	for _, entry := range entries {
		filePath := filepath.Join(workDir, entry.Name())
		if !entry.IsDir() && config.IsConfigFile(filePath) {
			configObject, err := config.ParseConfig(filePath)
			if err != nil {
				log.Fatalf("parsing config file %s failed error: %s", filePath, err)
			}
			resources, err := config.ProcessConfig(configObject, workDir)
			if err != nil {
				log.Fatalf("processing config file %s failed error: %s", filePath, err)
			}
			fmt.Println(resources)
		}
	}
}
