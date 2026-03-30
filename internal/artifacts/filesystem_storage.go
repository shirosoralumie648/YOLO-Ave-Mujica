package artifacts

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const artifactURIPrefix = "artifact://"

type FilesystemStorage struct {
	root string
}

func NewFilesystemStorage(root string) *FilesystemStorage {
	return &FilesystemStorage{root: root}
}

func (s *FilesystemStorage) StoreBuild(ctx context.Context, req StoreRequest) (StoredArtifact, error) {
	if s == nil || s.root == "" {
		return StoredArtifact{}, fmt.Errorf("filesystem storage root is not configured")
	}
	if req.Version == "" {
		return StoredArtifact{}, fmt.Errorf("version is required")
	}
	if req.ArchivePath == "" {
		return StoredArtifact{}, fmt.Errorf("archive path is required")
	}

	finalDir := filepath.Join(s.root, req.Version)
	tempDir := finalDir + ".tmp"
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return StoredArtifact{}, err
	}
	if _, err := os.Stat(finalDir); err == nil {
		return StoredArtifact{}, fmt.Errorf("artifact version %s already exists", req.Version)
	} else if !os.IsNotExist(err) {
		return StoredArtifact{}, err
	}

	_ = os.RemoveAll(tempDir)
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return StoredArtifact{}, err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	if req.PackageDir != "" {
		if err := copyDirContents(req.PackageDir, tempDir); err != nil {
			return StoredArtifact{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return StoredArtifact{}, err
	}

	archiveName := filepath.Base(req.ArchivePath)
	archiveFinalPath := filepath.Join(tempDir, archiveName)
	if err := copyFile(req.ArchivePath, archiveFinalPath); err != nil {
		return StoredArtifact{}, err
	}

	manifestName := "manifest.json"
	if req.ManifestPath != "" {
		manifestName = filepath.Base(req.ManifestPath)
		if err := copyFile(req.ManifestPath, filepath.Join(tempDir, manifestName)); err != nil {
			return StoredArtifact{}, err
		}
	}

	if err := os.Rename(tempDir, finalDir); err != nil {
		return StoredArtifact{}, err
	}

	archiveFinalPath = filepath.Join(finalDir, archiveName)
	info, err := os.Stat(archiveFinalPath)
	if err != nil {
		return StoredArtifact{}, err
	}

	manifestFinalPath := filepath.Join(finalDir, manifestName)
	archiveRelPath, err := filepath.Rel(s.root, archiveFinalPath)
	if err != nil {
		return StoredArtifact{}, err
	}
	manifestRelPath, err := filepath.Rel(s.root, manifestFinalPath)
	if err != nil {
		return StoredArtifact{}, err
	}

	return StoredArtifact{
		ArchivePath:  archiveFinalPath,
		ManifestPath: manifestFinalPath,
		ArchiveURI:   artifactURIPrefix + filepath.ToSlash(archiveRelPath),
		ManifestURI:  artifactURIPrefix + filepath.ToSlash(manifestRelPath),
		ArchiveSize:  info.Size(),
	}, nil
}

func (s *FilesystemStorage) OpenArchive(_ context.Context, uri string) (ReadSeekCloser, int64, error) {
	if s == nil || s.root == "" {
		return nil, 0, fmt.Errorf("filesystem storage root is not configured")
	}
	if !strings.HasPrefix(uri, artifactURIPrefix) {
		return nil, 0, fmt.Errorf("unsupported artifact uri %q", uri)
	}

	relPath := strings.TrimPrefix(uri, artifactURIPrefix)
	fullPath := filepath.Join(s.root, filepath.FromSlash(relPath))
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, 0, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, 0, err
	}
	return file, info.Size(), nil
}

func copyDirContents(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(srcPath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if srcPath == srcDir {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, srcPath)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, relPath)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		return copyFile(srcPath, dstPath)
	})
}

func copyFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	if err := dst.Chmod(info.Mode()); err != nil {
		return err
	}
	return dst.Close()
}
