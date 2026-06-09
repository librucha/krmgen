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
	var skipPatterns []string

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
			configPatterns := config.ReadSkipPatterns(srcDir)
			merged := mergeSkipPatterns(configPatterns, skipPatterns)
			workDir := copySrcDir(srcDir, merged)
			processWorkDir(workDir)
			defer func(path string) {
				_ = os.RemoveAll(path)
			}(workDir)
		},
	}

	command.Flags().StringArrayVar(&skipPatterns, "skip", nil, "glob pattern(s) of files to copy without template evaluation (e.g. *.pfx, assets/*.png)")

	return command
}

// mergeSkipPatterns combines config-level and CLI-level skip patterns, preserving order and removing duplicates.
func mergeSkipPatterns(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	var result []string
	for _, p := range append(a, b...) {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			result = append(result, p)
		}
	}
	return result
}

// matchesSkipPattern reports whether relPath matches any glob pattern.
// Each pattern is tested against both the full relative path and just the base filename,
// so "*.pfx" matches "certs/prod/cert.pfx" without needing a directory prefix.
func matchesSkipPattern(relPath string, patterns []string) bool {
	name := filepath.Base(relPath)
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
	}
	return false
}

func copySrcDir(srcDir string, skipPatterns []string) string {
	workDir, err := os.MkdirTemp(os.TempDir(), "krmgen")
	if err != nil {
		log.Fatalf("creating working dir in %s failed error: %s", os.TempDir(), err)
	}

	copyDir(srcDir, workDir, srcDir, skipPatterns)

	return workDir
}

func copyDir(srcDir string, dstDir string, baseDir string, skipPatterns []string) {
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
			copyDir(srcPath, dstPath, baseDir, skipPatterns)
		} else {
			fileContent, err := os.ReadFile(srcPath)
			if err != nil {
				log.Fatalf("reading file %s failed error: %s", srcPath, err)
			}
			relPath, _ := filepath.Rel(baseDir, srcPath)
			if matchesSkipPattern(relPath, skipPatterns) {
				err = os.WriteFile(dstPath, fileContent, os.ModePerm)
				if err != nil {
					log.Fatalf("writing file %s failed error: %s", srcPath, err)
				}
			} else {
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
