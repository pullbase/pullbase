package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/pullbase/pullbase/server/pkg/configvalidate"
)

func runValidateConfig(args []string) error {
	fs := flag.NewFlagSet("validate-config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	filePath := fs.String("file", "", "Path to config.yaml file to validate")
	output := fs.String("output", "table", "Output format: table or json")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*filePath) == "" {
		return errors.New("--file is required")
	}

	format := outputFormat(strings.ToLower(*output))
	if format != outputTable && format != outputJSON {
		return errors.New("--output must be 'table' or 'json'")
	}

	content, err := os.ReadFile(*filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	result := configvalidate.Validate(content)

	if format == outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if result.Valid {
		fmt.Println("✓ Config is valid")
		return nil
	}

	fmt.Printf("✗ Config has %d error(s):\n\n", len(result.Errors))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FIELD\tLINE\tMESSAGE")
	for _, e := range result.Errors {
		field := e.Field
		if field == "" {
			field = "(root)"
		}
		line := "-"
		if e.Line > 0 {
			line = fmt.Sprintf("%d", e.Line)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", field, line, e.Message)
	}
	w.Flush()

	return errors.New("validation failed")
}
