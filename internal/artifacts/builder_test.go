package artifacts

import (
	"bytes"
	"context"
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
	if !bytes.Contains(manifestBody, []byte(`"total_images": 1`)) || !bytes.Contains(manifestBody, []byte(`"total_annotations": 1`)) {
		t.Fatalf("manifest missing image or annotation stats: %s", manifestBody)
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
