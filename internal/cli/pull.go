package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// RootCommand is the minimal CLI dispatcher for MVP artifact delivery commands.
type RootCommand struct{}

// PullOptions configures artifact resolution, download, and local verification.
type PullOptions struct {
	Dataset        string
	Format         string
	Version        string
	AllowPartial   bool
	OutputDir      string
	ResolveTimeout time.Duration
	PollInterval   time.Duration
	VerifyWorkers  int
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
	ResolveArtifact(dataset, format, version string) (ResolvedArtifact, error)
	DownloadArchive(ctx context.Context, artifact ResolvedArtifact, tempPath string) error
}

type manifestDocument struct {
	Version string          `json:"version"`
	Entries []ManifestEntry `json:"entries"`
}

// ManifestEntry describes a single file and checksum in the pulled artifact manifest.
type ManifestEntry struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
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
  --dataset
  --format
  --version
  --allow-partial
  --wait-timeout
  --poll-interval
  --verify-workers
`
}

func runPull(args []string) error {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	dataset := fs.String("dataset", "", "dataset name")
	format := fs.String("format", "", "dataset format")
	version := fs.String("version", "", "dataset snapshot version")
	allowPartial := fs.Bool("allow-partial", false, "allow partial verification failures")
	waitTimeout := fs.Duration("wait-timeout", defaultResolveTimeout(), "max time to wait for artifact readiness")
	pollInterval := fs.Duration("poll-interval", defaultPollInterval(), "interval between artifact resolve retries")
	verifyWorkers := fs.Int("verify-workers", defaultVerifyWorkers(), "number of concurrent verification workers")
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
		Dataset:        *dataset,
		Format:         *format,
		Version:        *version,
		AllowPartial:   *allowPartial,
		OutputDir:      wd,
		ResolveTimeout: *waitTimeout,
		PollInterval:   *pollInterval,
		VerifyWorkers:  *verifyWorkers,
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
	if opts.ResolveTimeout < 0 {
		return fmt.Errorf("resolve timeout must be >= 0")
	}
	if opts.PollInterval < 0 {
		return fmt.Errorf("poll interval must be >= 0")
	}
	if opts.VerifyWorkers < 0 {
		return fmt.Errorf("verify workers must be >= 0")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	resolved, err := c.resolveArtifact(ctx, opts)
	if err != nil {
		return err
	}

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

	verificationWorkers := normalizeVerifyWorkers(opts.VerifyWorkers)
	fileResults, verifyErr := verifyManifestEntries(ctx, artifactDir, manifest.Entries, verificationWorkers, nil)
	failedFiles := 0
	verifiedFiles := 0
	for _, result := range fileResults {
		if result.Status == VerifyStatusFailed {
			failedFiles++
			continue
		}
		verifiedFiles++
	}

	report := VerifyReport{
		ArtifactID:          resolved.ArtifactID,
		Snapshot:            resolved.Version,
		TotalFiles:          len(manifest.Entries),
		VerifiedFiles:       verifiedFiles,
		FailedFiles:         failedFiles,
		VerificationWorkers: verificationWorkers,
		Files:               fileResults,
		VerifiedAt:          time.Now().UTC().Format(time.RFC3339),
		EnvironmentContext:  buildEnvironmentContext(c.source),
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

func (c *PullClient) resolveArtifact(ctx context.Context, opts PullOptions) (ResolvedArtifact, error) {
	waitCtx := ctx
	timeout := normalizeResolveTimeout(opts.ResolveTimeout)
	if timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var lastErr error
	for {
		resolved, err := c.source.ResolveArtifact(opts.Dataset, opts.Format, opts.Version)
		if err == nil {
			if resolved.Version == "" {
				resolved.Version = opts.Version
			}
			return resolved, nil
		}
		if !errors.Is(err, ErrArtifactUnavailable) {
			return ResolvedArtifact{}, err
		}
		lastErr = err

		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return ResolvedArtifact{}, fmt.Errorf("artifact %s/%s@%s did not become available within %s: %w", opts.Dataset, opts.Format, opts.Version, timeout, lastErr)
			}
			return ResolvedArtifact{}, waitCtx.Err()
		case <-time.After(normalizePollInterval(opts.PollInterval)):
		}
	}
}

func defaultResolveTimeout() time.Duration {
	return 30 * time.Second
}

func normalizeResolveTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultResolveTimeout()
	}
	return timeout
}

func defaultPollInterval() time.Duration {
	return 500 * time.Millisecond
}

func normalizePollInterval(interval time.Duration) time.Duration {
	if interval <= 0 {
		return defaultPollInterval()
	}
	return interval
}

func defaultVerifyWorkers() int {
	workers := runtime.NumCPU()
	if workers < 1 {
		return 1
	}
	if workers > 4 {
		return 4
	}
	return workers
}

func normalizeVerifyWorkers(workers int) int {
	if workers <= 0 {
		return defaultVerifyWorkers()
	}
	return workers
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
