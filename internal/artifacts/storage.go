package artifacts

import (
	"context"
	"io"
)

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

type StoreRequest struct {
	Version      string
	ArchivePath  string
	ManifestPath string
	PackageDir   string
}

type StoredArtifact struct {
	ArchivePath  string
	ManifestPath string
	ArchiveURI   string
	ManifestURI  string
	ArchiveSize  int64
}

type ArtifactStorage interface {
	StoreBuild(ctx context.Context, req StoreRequest) (StoredArtifact, error)
	OpenArchive(ctx context.Context, uri string) (ReadSeekCloser, int64, error)
}
