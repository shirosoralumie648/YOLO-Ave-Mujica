package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// RootCommand is the minimal CLI dispatcher for MVP artifact delivery commands.
type RootCommand struct{}

// PullOptions configures artifact resolution, download, and local verification.
type PullOptions struct {
	Format       string
	Version      string
	AllowPartial bool
	OutputDir    string
}

// PullClient resolves an artifact, downloads its archive, extracts it, and
// writes a local verification report for the pulled contents.
type PullClient struct {
	outputDir string
	source    ArtifactSource
}

// NewPullClient builds a pull client that writes output under the provided directory.
func NewPullClient(outputDir string) *PullClient {
	return NewPullClientWithSource(outputDir, nil)
}

// NewPullClientWithSource builds a pull client with an explicit artifact source implementation.
func NewPullClientWithSource(outputDir string, source ArtifactSource) *PullClient {
	return &PullClient{outputDir: outputDir, source: source}
}

// OutputDir returns the default extraction directory configured for this client.
func (c *PullClient) OutputDir() string {
	return c.outputDir
}

// ArtifactSource abstracts where pull fetches artifacts from so tests can
// replace the live HTTP implementation with deterministic fixtures.
type ArtifactSource interface {
	ResolveArtifact(format, version string) (ResolvedArtifact, error)
	DownloadArchive(ctx context.Context, artifact ResolvedArtifact, tempPath string) error
}

type manifestDocument struct {
	Version string          `json:"version"`
	Entries []ManifestEntry `json:"entries"`
}

// ManifestEntry describes a single file and checksum in the pulled artifact manifest.
type ManifestEntry struct {
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
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
		Format:       *format,
		Version:      *version,
		AllowPartial: *allowPartial,
		OutputDir:    wd,
	})
}

// Pull downloads, extracts, and verifies the requested artifact version.
func (c *PullClient) Pull(opts PullOptions) error {
	if opts.Format == "" || opts.Version == "" {
		return fmt.Errorf("format and version are required")
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

	resolved, err := c.source.ResolveArtifact(opts.Format, opts.Version)
	if err != nil {
		return err
	}
	if resolved.Version == "" {
		resolved.Version = opts.Version
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tempArchivePath := filepath.Join(outDir, fmt.Sprintf("pull-%s.tar.gz.part", resolved.Version))
	defer os.Remove(tempArchivePath)

	if err := c.source.DownloadArchive(ctx, resolved, tempArchivePath); err != nil {
		return err
	}

	artifactDir := filepath.Join(outDir, "pulled-"+resolved.Version)
	_ = os.RemoveAll(artifactDir)
	if err := extractTarGz(tempArchivePath, artifactDir); err != nil {
		return err
	}

	manifest, err := loadManifest(filepath.Join(artifactDir, "manifest.json"))
	if err != nil {
		return err
	}

	failedFiles := 0
	var verifyErr error
	for _, entry := range manifest.Entries {
		// Verify every manifest entry so local pulls can detect partial or corrupt
		// artifact contents before consumers start using the extracted package.
		targetPath := filepath.Join(artifactDir, filepath.FromSlash(entry.Path))
		if err := VerifyFile(targetPath, entry.Checksum); err != nil {
			failedFiles++
			if verifyErr == nil {
				verifyErr = err
			}
		}
	}

	report := VerifyReport{
		ArtifactID:         resolved.ArtifactID,
		Snapshot:           resolved.Version,
		TotalFiles:         len(manifest.Entries),
		FailedFiles:        failedFiles,
		VerifiedAt:         time.Now().UTC().Format(time.RFC3339),
		EnvironmentContext: buildEnvironmentContext(c.source),
	}
	reportPath := filepath.Join(outDir, "verify-report.json")
	if err := writeVerifyReport(reportPath, report); err != nil {
		return err
	}

	if verifyErr != nil && !opts.AllowPartial {
		return verifyErr
	}
	return nil
}

// loadManifest reads the extracted manifest document from disk.
func loadManifest(path string) (manifestDocument, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return manifestDocument{}, err
	}
	var manifest manifestDocument
	if err := json.Unmarshal(body, &manifest); err != nil {
		return manifestDocument{}, err
	}
	return manifest, nil
}
