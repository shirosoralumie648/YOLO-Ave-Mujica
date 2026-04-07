package artifacts

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

type StreamingObjectSource interface {
	OpenObject(ctx context.Context, objectKey string) (io.ReadCloser, error)
}

type ObjectSourceFunc func(context.Context, string) ([]byte, error)

func (f ObjectSourceFunc) ReadObject(ctx context.Context, objectKey string) ([]byte, error) {
	return f(ctx, objectKey)
}

type StreamingObjectSourceFunc func(context.Context, string) (io.ReadCloser, error)

func (f StreamingObjectSourceFunc) OpenObject(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	return f(ctx, objectKey)
}

func (f StreamingObjectSourceFunc) ReadObject(ctx context.Context, objectKey string) ([]byte, error) {
	reader, err := f.OpenObject(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
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
	format := bundle.Format
	if format == "" {
		format = "yolo"
	}

	rootDir := filepath.Join(workdir, "package")
	entries, totalAnnotations, err := b.writePackageContents(ctx, rootDir, bundle, format)
	if err != nil {
		return BuildOutput{}, err
	}

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

	archivePath := filepath.Join(workdir, fmt.Sprintf("package.%s.tar.gz", format))
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

func (b *Builder) writePackageContents(ctx context.Context, rootDir string, bundle ExportBundle, format string) ([]ManifestEntry, int, error) {
	switch format {
	case "coco":
		return b.writeCOCOPackage(ctx, rootDir, bundle)
	case "yolo":
		return b.writeYOLOPackage(ctx, rootDir, bundle)
	default:
		return nil, 0, fmt.Errorf("unsupported format: %s", format)
	}
}

func (b *Builder) writeYOLOPackage(ctx context.Context, rootDir string, bundle ExportBundle) ([]ManifestEntry, int, error) {
	imagesDir := filepath.Join(rootDir, "train", "images")
	labelsDir := filepath.Join(rootDir, "train", "labels")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return nil, 0, err
	}
	if err := os.MkdirAll(labelsDir, 0o755); err != nil {
		return nil, 0, err
	}

	entries := make([]ManifestEntry, 0, (len(bundle.Items)*2)+1)
	totalAnnotations := 0
	for _, item := range bundle.Items {
		imageDiskPath := filepath.Join(imagesDir, item.OutputName)
		imageChecksum, imageSize, err := b.writeObjectFile(ctx, item.ObjectKey, imageDiskPath)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, ManifestEntry{
			Path:     path.Join("train", "images", item.OutputName),
			Size:     imageSize,
			Checksum: imageChecksum,
		})

		labelBody := []byte(renderYOLOLabelFile(item.Boxes))
		totalAnnotations += len(item.Boxes)
		labelDiskPath := filepath.Join(labelsDir, item.LabelFileName)
		if err := os.WriteFile(labelDiskPath, labelBody, 0o644); err != nil {
			return nil, 0, err
		}
		entries = append(entries, ManifestEntry{
			Path:     path.Join("train", "labels", item.LabelFileName),
			Size:     int64(len(labelBody)),
			Checksum: checksumForBytes(labelBody),
		})
	}

	dataYAMLBody := []byte(BuildDataYAML("./train/images", "./train/images", bundle.Categories))
	dataYAMLPath := filepath.Join(rootDir, "data.yaml")
	if err := os.WriteFile(dataYAMLPath, dataYAMLBody, 0o644); err != nil {
		return nil, 0, err
	}
	entries = append(entries, ManifestEntry{
		Path:     "data.yaml",
		Size:     int64(len(dataYAMLBody)),
		Checksum: checksumForBytes(dataYAMLBody),
	})
	return entries, totalAnnotations, nil
}

func (b *Builder) writeCOCOPackage(ctx context.Context, rootDir string, bundle ExportBundle) ([]ManifestEntry, int, error) {
	imagesDir := filepath.Join(rootDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return nil, 0, err
	}

	entries := make([]ManifestEntry, 0, len(bundle.Items)+1)
	document := cocoDocument{
		Images:      make([]cocoImage, 0, len(bundle.Items)),
		Annotations: make([]cocoAnnotation, 0, bundle.TotalBoxes),
		Categories:  make([]cocoCategory, 0, len(bundle.Categories)),
	}
	for idx, name := range bundle.Categories {
		document.Categories = append(document.Categories, cocoCategory{
			ID:   categoryIDAt(bundle, idx),
			Name: name,
		})
	}

	totalAnnotations := 0
	nextAnnotationID := int64(1)
	for _, item := range bundle.Items {
		imageDiskPath := filepath.Join(imagesDir, item.OutputName)
		imageChecksum, imageSize, err := b.writeObjectFile(ctx, item.ObjectKey, imageDiskPath)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, ManifestEntry{
			Path:     path.Join("images", item.OutputName),
			Size:     imageSize,
			Checksum: imageChecksum,
		})
		document.Images = append(document.Images, cocoImage{
			ID:       item.ItemID,
			FileName: path.Join("images", item.OutputName),
			Width:    item.Width,
			Height:   item.Height,
		})
		for _, box := range item.Boxes {
			document.Annotations = append(document.Annotations, cocoAnnotation{
				ID:         nextAnnotationID,
				ImageID:    item.ItemID,
				CategoryID: cocoCategoryID(bundle, box),
				BBox:       []float64{box.BBoxX, box.BBoxY, box.BBoxW, box.BBoxH},
				Area:       box.BBoxW * box.BBoxH,
				IsCrowd:    0,
			})
			nextAnnotationID++
			totalAnnotations++
		}
	}

	annotationsBody, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, 0, err
	}
	annotationsPath := filepath.Join(rootDir, "annotations.json")
	if err := os.WriteFile(annotationsPath, annotationsBody, 0o644); err != nil {
		return nil, 0, err
	}
	entries = append(entries, ManifestEntry{
		Path:     "annotations.json",
		Size:     int64(len(annotationsBody)),
		Checksum: checksumForBytes(annotationsBody),
	})
	return entries, totalAnnotations, nil
}

func (b *Builder) writeObjectFile(ctx context.Context, objectKey, diskPath string) (string, int64, error) {
	if streamSource, ok := b.source.(StreamingObjectSource); ok {
		return b.streamObjectToFile(ctx, streamSource, objectKey, diskPath)
	}
	imageBody, err := b.readObject(ctx, objectKey)
	if err != nil {
		return "", 0, err
	}
	if err := os.WriteFile(diskPath, imageBody, 0o644); err != nil {
		return "", 0, err
	}
	return checksumForBytes(imageBody), int64(len(imageBody)), nil
}

func (b *Builder) readObject(ctx context.Context, objectKey string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	imageBody, err := b.source.ReadObject(ctx, objectKey)
	if err != nil {
		return nil, fmt.Errorf("read object %s: %w", objectKey, err)
	}
	return imageBody, nil
}

func (b *Builder) streamObjectToFile(ctx context.Context, source StreamingObjectSource, objectKey, diskPath string) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	reader, err := source.OpenObject(ctx, objectKey)
	if err != nil {
		return "", 0, fmt.Errorf("read object %s: %w", objectKey, err)
	}
	defer reader.Close()

	file, err := os.OpenFile(diskPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", 0, err
	}
	size, err := io.Copy(file, reader)
	if err != nil {
		file.Close()
		return "", 0, fmt.Errorf("read object %s: %w", objectKey, err)
	}
	if err := file.Close(); err != nil {
		return "", 0, err
	}
	checksum, err := fileChecksum(diskPath)
	if err != nil {
		return "", 0, err
	}
	return checksum, size, nil
}

type cocoDocument struct {
	Images      []cocoImage      `json:"images"`
	Annotations []cocoAnnotation `json:"annotations"`
	Categories  []cocoCategory   `json:"categories"`
}

type cocoImage struct {
	ID       int64  `json:"id"`
	FileName string `json:"file_name"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type cocoAnnotation struct {
	ID         int64     `json:"id"`
	ImageID    int64     `json:"image_id"`
	CategoryID int64     `json:"category_id"`
	BBox       []float64 `json:"bbox"`
	Area       float64   `json:"area"`
	IsCrowd    int       `json:"iscrowd"`
}

type cocoCategory struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func categoryIDAt(bundle ExportBundle, index int) int64 {
	if index >= 0 && index < len(bundle.CategoryIDs) && bundle.CategoryIDs[index] > 0 {
		return bundle.CategoryIDs[index]
	}
	return int64(index + 1)
}

func cocoCategoryID(bundle ExportBundle, box YOLOBox) int64 {
	if box.CategoryID > 0 {
		return box.CategoryID
	}
	if box.ClassIndex >= 0 && box.ClassIndex < len(bundle.CategoryIDs) {
		return categoryIDAt(bundle, box.ClassIndex)
	}
	return int64(box.ClassIndex + 1)
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
