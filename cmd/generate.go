package cmd

import (
	"fmt"
	"github.com/librucha/krmgen/internal/config"
	"github.com/librucha/krmgen/internal/redact"
	"github.com/librucha/krmgen/internal/template"
	cons "github.com/librucha/krmgen/internal/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
			// Last stop before cobra prints the error. A kustomization may
			// pull a remote base over HTTPS with the credential embedded in
			// the URL - typically resolved from a key vault by a template -
			// and kustomize echoes that URL verbatim into its error, twice:
			// once naming the resource it could not accumulate, once quoting
			// the git command line it ran.
			return redact.Error(generate(srcDir, merged))
		},
	}

	command.Flags().StringArrayVar(&skipPatterns, "skip", nil, "glob pattern(s) of files to copy without template evaluation (e.g. *.pfx, assets/*.png)")

	return command
}

// removeAll is a seam: tests replace it to observe the working-directory
// cleanup-failure warning without needing a real read-only mount, following
// the same pattern as runCommand in internal/helm/processor.go.
var removeAll = os.RemoveAll

// generate owns the working directory for its whole lifetime: the deferred
// removal is registered immediately after the directory exists, so it runs on
// every path out - including a failure part-way through processing, which
// used to leave a directory full of rendered secrets behind.
//
// A cleanup failure itself does not turn a successful render into a failing
// run: for krmgen used as an ArgoCD CMP plugin, that distinction is "sync OK"
// vs. "sync failed", and failing the run does not get the directory removed
// either. Instead it is reported as a stderr warning naming the path left
// behind, since it may still hold rendered secrets.
func generate(srcDir string, skipPatterns []string) (err error) {
	values, err := config.ResolveValues(srcDir)
	if err != nil {
		return err
	}
	data := map[string]any{config.ValuesKey: values}
	workDir, err := copySrcDir(srcDir, skipPatterns, data)
	if workDir != "" {
		defer func() {
			if rmErr := removeAll(workDir); rmErr != nil {
				fmt.Fprintf(os.Stderr, "warning: could not remove working dir %s: %v; it may contain rendered secrets\n", workDir, rmErr)
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

func copySrcDir(srcDir string, skipPatterns []string, data map[string]any) (string, error) {
	workDir, err := os.MkdirTemp(os.TempDir(), "krmgen")
	if err != nil {
		return "", fmt.Errorf("creating working dir in %s failed error: %w", os.TempDir(), err)
	}

	if err := copyDir(srcDir, workDir, srcDir, skipPatterns, data); err != nil {
		return workDir, err
	}

	return workDir, nil
}

func copyDir(srcDir string, dstDir string, baseDir string, skipPatterns []string, data map[string]any) error {
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
			if err := copyDir(srcPath, dstPath, baseDir, skipPatterns, data); err != nil {
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
			if !strings.ContainsRune(relPath, filepath.Separator) {
				// values were already resolved; never render them twice
				fileContent = config.StripValues(fileContent)
			}
			// evaluate templates
			evaluated, err := template.EvalGoTemplates(string(fileContent), data)
			if err != nil {
				return fmt.Errorf("template evaluation of file %s failed error: %w%s", srcPath, err, missingValueHint(err))
			}
			if err := os.WriteFile(dstPath, []byte(evaluated), cons.FilePerm); err != nil {
				return fmt.Errorf("writing evaluated file %s failed error: %w", srcPath, err)
			}
		}
	}
	return nil
}

// missingValueHint explains a missing .Values key: either a typo in the
// name, or a templated value left unquoted, which makes the KrmGen file
// invalid YAML before templating, so ResolveValues skips it.
func missingValueHint(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "map has no entry for key") && strings.Contains(msg, ".Values") {
		return " (key not defined in values: - values are read from KrmGen files before templating; check the name and quoting)"
	}
	return ""
}

func processWorkDir(workDir string) error {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return fmt.Errorf("reading work directory %s failed error: %w", workDir, err)
	}

	for _, entry := range entries {
		filePath := filepath.Join(workDir, entry.Name())
		if entry.IsDir() {
			continue
		}
		if err := checkConfigYAML(filePath); err != nil {
			return fmt.Errorf("config file %s is not valid YAML after templating: %w", entry.Name(), err)
		}
		if config.IsConfigFile(filePath) {
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

// krmGenKindLine matches a top-level kind: KrmGen line in raw content.
var krmGenKindLine = regexp.MustCompile(`(?m)^kind:\s*["']?KrmGen["']?\s*$`)

// checkConfigYAML returns the YAML error of a file that declares kind: KrmGen
// but does not parse. IsConfigFile treats such a file as "not a config" and
// skipping it would silently empty the output.
func checkConfigYAML(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil || !krmGenKindLine.Match(content) {
		return nil
	}
	var contentObject map[string]any
	return yaml.Unmarshal(content, &contentObject)
}
