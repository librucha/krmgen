package cmd

import (
	"fmt"
	"github.com/librucha/krmgen/internal/config"
	"github.com/librucha/krmgen/internal/template"
	cons "github.com/librucha/krmgen/internal/utils"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

func NewGenerateCommand() *cobra.Command {
	var skipPatterns []string

	command := &cobra.Command{
		Use:          "generate <path>",
		Short:        "Generate KRM by declared config",
		Aliases:      []string{"g"},
		SilenceUsage: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("<path> argument required to generate KRM")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			srcDir, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			configPatterns := config.ReadSkipPatterns(srcDir)
			merged := mergeSkipPatterns(configPatterns, skipPatterns)
			return generate(srcDir, merged)
		},
	}

	command.Flags().StringArrayVar(&skipPatterns, "skip", nil, "glob pattern(s) of files to copy without template evaluation (e.g. *.pfx, assets/*.png)")

	return command
}

// generate owns the working directory for its whole lifetime: the deferred
// removal is registered immediately after the directory exists, so it runs on
// every path out - including a failure part-way through processing, which
// used to leave a directory full of rendered secrets behind.
func generate(srcDir string, skipPatterns []string) (err error) {
	workDir, err := copySrcDir(srcDir, skipPatterns)
	if workDir != "" {
		defer func() {
			if rmErr := os.RemoveAll(workDir); rmErr != nil && err == nil {
				err = fmt.Errorf("removing working dir %s failed error: %w", workDir, rmErr)
			}
		}()
	}
	if err != nil {
		return err
	}
	return processWorkDir(workDir)
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

func copySrcDir(srcDir string, skipPatterns []string) (string, error) {
	workDir, err := os.MkdirTemp(os.TempDir(), "krmgen")
	if err != nil {
		return "", fmt.Errorf("creating working dir in %s failed error: %w", os.TempDir(), err)
	}

	if err := copyDir(srcDir, workDir, srcDir, skipPatterns); err != nil {
		return workDir, err
	}

	return workDir, nil
}

func copyDir(srcDir string, dstDir string, baseDir string, skipPatterns []string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("reading source directory %s failed error: %w", srcDir, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, cons.DirPerm); err != nil {
				return fmt.Errorf("creating directory %s failed error: %w", dstPath, err)
			}
			if err := copyDir(srcPath, dstPath, baseDir, skipPatterns); err != nil {
				return err
			}
			continue
		}

		fileContent, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("reading file %s failed error: %w", srcPath, err)
		}
		relPath, _ := filepath.Rel(baseDir, srcPath)
		if matchesSkipPattern(relPath, skipPatterns) {
			if err := os.WriteFile(dstPath, fileContent, cons.FilePerm); err != nil {
				return fmt.Errorf("writing file %s failed error: %w", srcPath, err)
			}
		} else {
			// evaluate templates
			evaluated, err := template.EvalGoTemplates(string(fileContent))
			if err != nil {
				return fmt.Errorf("template evaluation of file %s failed error: %w", srcPath, err)
			}
			if err := os.WriteFile(dstPath, []byte(evaluated), cons.FilePerm); err != nil {
				return fmt.Errorf("writing evaluated file %s failed error: %w", srcPath, err)
			}
		}
	}
	return nil
}

func processWorkDir(workDir string) error {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return fmt.Errorf("reading work directory %s failed error: %w", workDir, err)
	}

	for _, entry := range entries {
		filePath := filepath.Join(workDir, entry.Name())
		if !entry.IsDir() && config.IsConfigFile(filePath) {
			configObject, err := config.ParseConfig(filePath)
			if err != nil {
				return fmt.Errorf("parsing config file %s failed error: %w", filePath, err)
			}
			resources, err := config.ProcessConfig(configObject, workDir)
			if err != nil {
				return fmt.Errorf("processing config file %s failed error: %w", filePath, err)
			}
			fmt.Println(resources)
		}
	}
	return nil
}
