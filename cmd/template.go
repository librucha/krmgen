package cmd

import (
	"fmt"
	"strings"

	"github.com/librucha/krmgen/v2/internal/config"
	"github.com/librucha/krmgen/v2/internal/redact"
	"github.com/librucha/krmgen/v2/internal/template"
	"github.com/spf13/cobra"
)

func NewTemplateCommand() *cobra.Command {
	var vars []string
	// A string array although only one file is accepted: with a plain string
	// flag a repeated --var-file would silently keep the last one and drop
	// the variables of the others.
	var varFiles []string

	command := &cobra.Command{
		Use:   "template <template>",
		Short: "Evaluate a Go template string and print the result",
		Long: `Evaluate a Go template string with the template functions krmgen offers in
config files, and print the result to stdout exactly as evaluated - no newline
is added.

Variables are available under .Values, as they are in files rendered by
"krmgen generate". They come from --var-file (a mapping in a .yaml, .yml or
.json file) and from --var key=value, which overrides the file. A --var value
is always a string; a dotted key such as db.host sets a nested value.

Put -- before a template that starts with a dash.`,
		Example: `  krmgen template '{{ krmgenGenerated }}'
  krmgen template --var ns='black widow' '{{ upper .Values.ns }}'
  krmgen template --var-file variables.yaml --var env=prod '{{ .Values.ns }}/{{ .Values.env }}'`,
		SilenceUsage: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("exactly one <template> argument required, got %d", len(args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Same last stop as generate: a template function may fail with
			// an error that quotes a URL carrying a credential.
			return redact.Error(evalTemplate(cmd, args[0], varFiles, vars))
		},
	}

	// A flag-parsing error never reaches RunE, so it gets its masking here.
	// It matters because the most likely one quotes the template itself: a
	// template that starts with a dash and was passed without -- is parsed
	// as a flag ("unknown shorthand flag: 'x' in -x...").
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		if strings.HasPrefix(err.Error(), "unknown ") {
			err = fmt.Errorf("%w (a template that starts with a dash must follow --)", err)
		}
		return redact.Error(err)
	})

	// StringArray, not StringSlice: the slice variant splits a value on
	// commas, turning --var hosts=a,b into two assignments.
	// The back-quoted word is what pflag prints as the value placeholder in
	// --help, instead of the type name "stringArray".
	command.Flags().StringArrayVar(&vars, "var", nil, "variable as `key=value`, exposed as .Values.key; repeatable, overrides --var-file")
	command.Flags().StringArrayVar(&varFiles, "var-file", nil, "`file` with variables exposed under .Values (.yaml, .yml or .json); one file only")

	return command
}

func evalTemplate(cmd *cobra.Command, content string, varFiles []string, vars []string) error {
	if len(varFiles) > 1 {
		return fmt.Errorf("--var-file given %d times, only one variables file is supported", len(varFiles))
	}
	varFile := ""
	if len(varFiles) == 1 {
		varFile = varFiles[0]
		// LoadVars reads "" as "no file". Here the flag was given, so an
		// empty path is a mistake - typically --var-file "$VARS" with VARS
		// unset - and evaluating without the variables would hide it.
		if varFile == "" {
			return fmt.Errorf("--var-file is given an empty path")
		}
	}
	values, err := config.LoadVars(varFile, vars)
	if err != nil {
		return err
	}
	evaluated, err := template.EvalGoTemplates(content, map[string]any{config.ValuesKey: values})
	if err != nil {
		return fmt.Errorf("template evaluation failed error: %w%s", err, missingVarHint(err))
	}
	// Fprint, not Fprintln: the output is the evaluated text byte for byte.
	_, err = fmt.Fprint(cmd.OutOrStdout(), evaluated)
	return err
}

// missingVarHint points at the two flags when the template reads a .Values
// key nobody passed. generate has its own hint for the same template error
// (missingValueHint), but that one talks about KrmGen files.
func missingVarHint(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "map has no entry for key") && strings.Contains(msg, ".Values") {
		return " (variable not passed: use --var key=value or --var-file)"
	}
	return ""
}
