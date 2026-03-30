package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func downloadArchiveToTemp(ctx context.Context, client *http.Client, targetURL, tempPath string) error {
	if client == nil {
		client = http.DefaultClient
	}

	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	startOffset := info.Size()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return err
	}
	if startOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		if startOffset > 0 {
			if err := file.Truncate(0); err != nil {
				return err
			}
			startOffset = 0
		}
	case http.StatusPartialContent:
	case http.StatusRequestedRangeNotSatisfiable:
		if err := file.Truncate(0); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		return downloadArchiveToTemp(ctx, client, targetURL, tempPath)
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d for %s: %s", resp.StatusCode, targetURL, string(body))
	}

	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		return err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		return err
	}
	return nil
}

func extractTarGz(archivePath, outDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		targetPath, err := safeExtractPath(outDir, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			if err := outFile.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported tar entry %s (%d)", header.Name, header.Typeflag)
		}
	}
}

func safeExtractPath(outDir, entryName string) (string, error) {
	cleanPath := filepath.Clean(filepath.Join(outDir, filepath.FromSlash(entryName)))
	rootWithSep := outDir + string(os.PathSeparator)
	if cleanPath != outDir && !strings.HasPrefix(cleanPath, rootWithSep) {
		return "", fmt.Errorf("archive entry escapes output dir: %s", entryName)
	}
	return cleanPath, nil
}
