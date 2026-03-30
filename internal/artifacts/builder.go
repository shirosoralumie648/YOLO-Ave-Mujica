package artifacts

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type ObjectSource interface {
	ReadObject(ctx context.Context, objectKey string) ([]byte, error)
}

type ObjectSourceFunc func(context.Context, string) ([]byte, error)

func (f ObjectSourceFunc) ReadObject(ctx context.Context, objectKey string) ([]byte, error) {
	return f(ctx, objectKey)
}

type BuildOutput struct {
	RootDir       string
	ManifestPath  string
	ArchivePath   string
	ArchiveSHA256 string
	ArchiveSize   int64
}

type Builder struct {
	source ObjectSource
}

func NewBuilder(source ObjectSource) *Builder {
	return &Builder{source: source}
}

func (b *Builder) Build(ctx context.Context, workdir string, bundle ExportBundle) (BuildOutput, error) {
	if b == nil || b.source == nil {
		return BuildOutput{}, fmt.Errorf("artifact builder source is not configured")
	}

	rootDir := filepath.Join(workdir, "package")
	imagesDir := filepath.Join(rootDir, "train", "images")
	labelsDir := filepath.Join(rootDir, "train", "labels")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return BuildOutput{}, err
	}
	if err := os.MkdirAll(labelsDir, 0o755); err != nil {
		return BuildOutput{}, err
	}

	entries := make([]ManifestEntry, 0, (len(bundle.Items)*2)+1)
	totalAnnotations := 0
	for _, item := range bundle.Items {
		if err := ctx.Err(); err != nil {
			return BuildOutput{}, err
		}

		imageBody, err := b.source.ReadObject(ctx, item.ObjectKey)
		if err != nil {
			return BuildOutput{}, fmt.Errorf("read object %s: %w", item.ObjectKey, err)
		}
		imageDiskPath := filepath.Join(imagesDir, item.OutputName)
		if err := os.WriteFile(imageDiskPath, imageBody, 0o644); err != nil {
			return BuildOutput{}, err
		}
		entries = append(entries, ManifestEntry{
			Path:     path.Join("train", "images", item.OutputName),
			Checksum: checksumForBytes(imageBody),
		})

		labelBody := []byte(renderYOLOLabelFile(item.Boxes))
		totalAnnotations += len(item.Boxes)
		labelDiskPath := filepath.Join(labelsDir, item.LabelFileName)
		if err := os.WriteFile(labelDiskPath, labelBody, 0o644); err != nil {
			return BuildOutput{}, err
		}
		entries = append(entries, ManifestEntry{
			Path:     path.Join("train", "labels", item.LabelFileName),
			Checksum: checksumForBytes(labelBody),
		})
	}

	dataYAMLBody := []byte(BuildDataYAML("./train/images", "./train/images", bundle.Categories))
	dataYAMLPath := filepath.Join(rootDir, "data.yaml")
	if err := os.WriteFile(dataYAMLPath, dataYAMLBody, 0o644); err != nil {
		return BuildOutput{}, err
	}
	entries = append(entries, ManifestEntry{
		Path:     "data.yaml",
		Checksum: checksumForBytes(dataYAMLBody),
	})

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	manifestPath := filepath.Join(rootDir, "manifest.json")
	manifestBody, err := BuildManifestWithMetadata(bundle.Version, buildManifestCategoryMap(bundle.Categories), ManifestStats{
		TotalImages:      len(bundle.Items),
		TotalAnnotations: totalAnnotations,
		TotalClasses:     len(bundle.Categories),
	}, entries)
	if err != nil {
		return BuildOutput{}, err
	}
	if err := os.WriteFile(manifestPath, manifestBody, 0o644); err != nil {
		return BuildOutput{}, err
	}

	archivePath := filepath.Join(workdir, "package.yolo.tar.gz")
	if err := tarGzDir(rootDir, archivePath); err != nil {
		return BuildOutput{}, err
	}

	info, err := os.Stat(archivePath)
	if err != nil {
		return BuildOutput{}, err
	}
	archiveChecksum, err := fileChecksum(archivePath)
	if err != nil {
		return BuildOutput{}, err
	}

	return BuildOutput{
		RootDir:       rootDir,
		ManifestPath:  manifestPath,
		ArchivePath:   archivePath,
		ArchiveSHA256: archiveChecksum,
		ArchiveSize:   info.Size(),
	}, nil
}

func buildManifestCategoryMap(categories []string) map[string]string {
	categoryMap := make(map[string]string, len(categories))
	for i, name := range categories {
		categoryMap[strconv.Itoa(i)] = name
	}
	return categoryMap
}

func renderYOLOLabelFile(boxes []YOLOBox) string {
	if len(boxes) == 0 {
		return ""
	}
	out := ""
	for _, box := range boxes {
		out += fmt.Sprintf("%d %.6f %.6f %.6f %.6f\n", box.ClassIndex, box.XCenter, box.YCenter, box.Width, box.Height)
	}
	return out
}

func tarGzDir(rootDir, archivePath string) error {
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	gzipWriter := gzip.NewWriter(archiveFile)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	files := make([]string, 0)
	if err := filepath.Walk(rootDir, func(filePath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, filePath)
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(files)

	zeroTime := time.Unix(0, 0).UTC()
	for _, filePath := range files {
		relPath, err := filepath.Rel(rootDir, filePath)
		if err != nil {
			return err
		}
		info, err := os.Stat(filePath)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)
		header.ModTime = zeroTime
		header.AccessTime = zeroTime
		header.ChangeTime = zeroTime
		header.Uid = 0
		header.Gid = 0
		header.Uname = ""
		header.Gname = ""

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tarWriter, file); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func checksumForBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return NormalizeSHA256Checksum(hex.EncodeToString(sum[:]))
}

func fileChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return NormalizeSHA256Checksum(hex.EncodeToString(hash.Sum(nil))), nil
}
