package artifacts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBuilderWritesTrainLayoutAndManifestStats(t *testing.T) {
	workdir := t.TempDir()
	builder := NewBuilder(fakeObjectSource{
		"train/a.jpg": []byte("fake-image-a"),
	})

	bundle := ExportBundle{
		Version:    "v1",
		Categories: []string{"person"},
		Items: []ExportItem{
			{
				ObjectKey:     "train/a.jpg",
				OutputName:    "a.jpg",
				LabelFileName: "a.txt",
				Boxes: []YOLOBox{
					{ClassIndex: 0, XCenter: 0.5, YCenter: 0.5, Width: 0.2, Height: 0.2},
				},
			},
		},
	}

	out, err := builder.Build(context.Background(), workdir, bundle)
	if err != nil {
		t.Fatalf("build package: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out.RootDir, "train", "images", "a.jpg")); err != nil {
		t.Fatalf("missing train image: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out.RootDir, "train", "labels", "a.txt")); err != nil {
		t.Fatalf("missing train label: %v", err)
	}
	manifestBody, err := os.ReadFile(filepath.Join(out.RootDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !bytes.Contains(manifestBody, []byte(`"category_map"`)) || !bytes.Contains(manifestBody, []byte(`"sha256:`)) {
		t.Fatalf("manifest missing category_map or sha256 checksums: %s", manifestBody)
	}
	if !bytes.Contains(manifestBody, []byte(`"size": 12`)) {
		t.Fatalf("manifest missing image size metadata: %s", manifestBody)
	}
	if !bytes.Contains(manifestBody, []byte(`"total_images": 1`)) || !bytes.Contains(manifestBody, []byte(`"total_annotations": 1`)) {
		t.Fatalf("manifest missing image or annotation stats: %s", manifestBody)
	}
}

func TestBuilderWritesCOCOLayoutAndAnnotationDocument(t *testing.T) {
	workdir := t.TempDir()
	builder := NewBuilder(fakeObjectSource{
		"train/a.jpg": []byte("fake-image-a"),
	})

	bundle := ExportBundle{
		Format:      "coco",
		Version:     "v2",
		Categories:  []string{"person"},
		CategoryIDs: []int64{7},
		Items: []ExportItem{
			{
				ItemID:        41,
				ObjectKey:     "train/a.jpg",
				OutputName:    "a.jpg",
				LabelFileName: "a.txt",
				Width:         1280,
				Height:        720,
				Boxes: []YOLOBox{
					{
						CategoryID: 7,
						ClassIndex: 0,
						BBoxX:      10,
						BBoxY:      20,
						BBoxW:      30,
						BBoxH:      40,
						XCenter:    25,
						YCenter:    40,
						Width:      30,
						Height:     40,
					},
				},
			},
		},
	}

	out, err := builder.Build(context.Background(), workdir, bundle)
	if err != nil {
		t.Fatalf("build coco package: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out.RootDir, "images", "a.jpg")); err != nil {
		t.Fatalf("missing coco image: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out.RootDir, "annotations.json")); err != nil {
		t.Fatalf("missing coco annotations: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out.RootDir, "data.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected coco package to omit data.yaml, got err=%v", err)
	}
	manifestBody, err := os.ReadFile(filepath.Join(out.RootDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !bytes.Contains(manifestBody, []byte(`"path": "annotations.json"`)) {
		t.Fatalf("manifest missing annotations.json entry: %s", manifestBody)
	}
	if !bytes.Contains(manifestBody, []byte(`"total_annotations": 1`)) {
		t.Fatalf("manifest missing annotation stats: %s", manifestBody)
	}
	annotationsBody, err := os.ReadFile(filepath.Join(out.RootDir, "annotations.json"))
	if err != nil {
		t.Fatalf("read annotations: %v", err)
	}
	if !bytes.Contains(annotationsBody, []byte(`"file_name": "images/a.jpg"`)) {
		t.Fatalf("expected coco file_name, got %s", annotationsBody)
	}
	if !bytes.Contains(annotationsBody, []byte(`"category_id": 7`)) {
		t.Fatalf("expected source category id in coco export, got %s", annotationsBody)
	}
	if !bytes.Contains(annotationsBody, []byte(`"bbox": [`)) {
		t.Fatalf("expected bbox in coco export, got %s", annotationsBody)
	}
	if filepath.Base(out.ArchivePath) != "package.coco.tar.gz" {
		t.Fatalf("expected coco archive name, got %s", out.ArchivePath)
	}
}

func TestBuilderPrefersStreamingObjectReadsWhenAvailable(t *testing.T) {
	workdir := t.TempDir()
	source := streamingPreferredObjectSource{
		bodies: map[string][]byte{
			"train/a.jpg": []byte("fake-image-a"),
		},
	}
	builder := NewBuilder(&source)

	bundle := ExportBundle{
		Version:    "v1",
		Categories: []string{"person"},
		Items: []ExportItem{
			{
				ObjectKey:     "train/a.jpg",
				OutputName:    "a.jpg",
				LabelFileName: "a.txt",
				Boxes: []YOLOBox{
					{ClassIndex: 0, XCenter: 0.5, YCenter: 0.5, Width: 0.2, Height: 0.2},
				},
			},
		},
	}

	out, err := builder.Build(context.Background(), workdir, bundle)
	if err != nil {
		t.Fatalf("build package: %v", err)
	}
	if source.readCalls != 0 {
		t.Fatalf("expected builder to avoid ReadObject when OpenObject is available, got %d read calls", source.readCalls)
	}
	if source.openCalls != 1 {
		t.Fatalf("expected builder to open the object once, got %d", source.openCalls)
	}
	imageBody, err := os.ReadFile(filepath.Join(out.RootDir, "train", "images", "a.jpg"))
	if err != nil {
		t.Fatalf("read streamed image: %v", err)
	}
	if string(imageBody) != "fake-image-a" {
		t.Fatalf("expected streamed image contents, got %q", string(imageBody))
	}
}

type fakeObjectSource map[string][]byte

func (f fakeObjectSource) ReadObject(_ context.Context, objectKey string) ([]byte, error) {
	body, ok := f[objectKey]
	if !ok {
		return nil, os.ErrNotExist
	}
	return body, nil
}

type streamingPreferredObjectSource struct {
	bodies    map[string][]byte
	openCalls int
	readCalls int
}

func (s *streamingPreferredObjectSource) OpenObject(_ context.Context, objectKey string) (io.ReadCloser, error) {
	body, ok := s.bodies[objectKey]
	if !ok {
		return nil, os.ErrNotExist
	}
	s.openCalls++
	return io.NopCloser(bytes.NewReader(body)), nil
}

func (s *streamingPreferredObjectSource) ReadObject(_ context.Context, objectKey string) ([]byte, error) {
	s.readCalls++
	return nil, fmt.Errorf("ReadObject should not be used for %s", objectKey)
}
