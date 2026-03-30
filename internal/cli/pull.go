package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type RootCommand struct{}

type PullOptions struct {
	Dataset      string
	Format       string
	Version      string
	AllowPartial bool
	OutputDir    string
}

type PullClient struct {
	outputDir string
	source    ArtifactSource
}

func NewPullClient(outputDir string) *PullClient {
	return NewPullClientWithSource(outputDir, nil)
}

func NewPullClientWithSource(outputDir string, source ArtifactSource) *PullClient {
	return &PullClient{outputDir: outputDir, source: source}
}

func (c *PullClient) OutputDir() string {
	return c.outputDir
}

type ArtifactEntry struct {
	Path     string
	Body     []byte
	Checksum string
}

type PulledArtifact struct {
	ArtifactID int64
	Version    string
	Entries    []ArtifactEntry
}

type ArtifactSource interface {
	FetchArtifact(dataset, format, version string) (PulledArtifact, error)
}

// NewRootCommand returns a minimal command dispatcher for MVP CLI flows.
func NewRootCommand() *RootCommand {
	return &RootCommand{}
}

// Execute dispatches to subcommands and provides top-level help.
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
  --dataset
  --format
  --version
  --allow-partial
`
}

func runPull(args []string) error {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	dataset := fs.String("dataset", "", "dataset name")
	format := fs.String("format", "", "dataset format")
	version := fs.String("version", "", "dataset snapshot version")
	allowPartial := fs.Bool("allow-partial", false, "allow partial verification failures")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *dataset == "" || *format == "" || *version == "" {
		return fmt.Errorf("--dataset, --format and --version are required")
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	baseURL := os.Getenv("API_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	client := NewPullClientWithSource(wd, NewAPIArtifactSource(baseURL))
	return client.Pull(PullOptions{
		Dataset:      *dataset,
		Format:       *format,
		Version:      *version,
		AllowPartial: *allowPartial,
		OutputDir:    wd,
	})
}

func (c *PullClient) Pull(opts PullOptions) error {
	if opts.Dataset == "" || opts.Format == "" || opts.Version == "" {
		return fmt.Errorf("dataset, format and version are required")
	}
	if c.source == nil {
		return fmt.Errorf("artifact source is not configured")
	}

	outDir := opts.OutputDir
	if outDir == "" {
		outDir = c.outputDir
	}
	if outDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		outDir = wd
	}

	pulled, err := c.source.FetchArtifact(opts.Dataset, opts.Format, opts.Version)
	if err != nil {
		return err
	}
	if pulled.Version == "" {
		pulled.Version = opts.Version
	}

	artifactDir := filepath.Join(outDir, "pulled-"+pulled.Version)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}

	failedFiles := 0
	var verifyErr error
	manifestEntries := make([]map[string]any, 0, len(pulled.Entries))
	for _, entry := range pulled.Entries {
		targetPath := filepath.Join(artifactDir, entry.Path)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, entry.Body, 0o644); err != nil {
			return err
		}
		if err := VerifyFile(targetPath, entry.Checksum); err != nil {
			failedFiles++
			if verifyErr == nil {
				verifyErr = err
			}
		}
		manifestEntries = append(manifestEntries, map[string]any{
			"path":     entry.Path,
			"checksum": entry.Checksum,
		})
	}

	report := VerifyReport{
		ArtifactID:         pulled.ArtifactID,
		Snapshot:           pulled.Version,
		TotalFiles:         len(pulled.Entries),
		FailedFiles:        failedFiles,
		VerifiedAt:         time.Now().UTC().Format(time.RFC3339),
		EnvironmentContext: buildEnvironmentContext(c.source),
	}
	reportPath := filepath.Join(outDir, "verify-report.json")
	if err := writeVerifyReport(reportPath, report); err != nil {
		return err
	}

	manifestPath := filepath.Join(artifactDir, "manifest.json")
	manifestPayload, _ := json.MarshalIndent(map[string]any{
		"version": pulled.Version,
		"entries": manifestEntries,
	}, "", "  ")
	if err := os.WriteFile(manifestPath, manifestPayload, 0o644); err != nil {
		return err
	}

	if verifyErr != nil && !opts.AllowPartial {
		return verifyErr
	}
	return nil
}
