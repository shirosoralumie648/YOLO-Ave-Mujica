package cli

import (
	"flag"
	"fmt"
	"os"
)

type RootCommand struct{}

func NewRootCommand() *RootCommand {
	return &RootCommand{}
}

func (r *RootCommand) Execute() error {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Print(helpText())
		return nil
	}

	switch args[0] {
	case "pull":
		return runPull(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func helpText() string {
	return `platform-cli

Commands:
  pull

Flags for pull:
  --format
  --version
  --allow-partial
`
}

func runPull(args []string) error {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	format := fs.String("format", "", "dataset format")
	version := fs.String("version", "", "dataset snapshot version")
	allowPartial := fs.Bool("allow-partial", false, "allow partial verification failures")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *format == "" || *version == "" {
		return fmt.Errorf("--format and --version are required")
	}
	_ = allowPartial
	return nil
}
