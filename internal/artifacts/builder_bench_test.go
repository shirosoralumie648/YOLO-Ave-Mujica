package artifacts

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func BenchmarkBuilderBuildPackage(b *testing.B) {
	body := bytes.Repeat([]byte("image-bytes-"), 8192)
	source := benchmarkStreamingObjectSource{body: body}
	builder := NewBuilder(source)
	ctx := context.Background()

	for _, format := range []string{"yolo", "coco"} {
		b.Run(format, func(b *testing.B) {
			b.ReportAllocs()
			root := b.TempDir()
			bundle := benchmarkExportBundle(format, 8)
			for i := 0; i < b.N; i++ {
				workdir := filepath.Join(root, strconv.Itoa(i))
				if err := os.MkdirAll(workdir, 0o755); err != nil {
					b.Fatalf("mkdir workdir: %v", err)
				}
				if _, err := builder.Build(ctx, workdir, bundle); err != nil {
					b.Fatalf("build package: %v", err)
				}
				if err := os.RemoveAll(workdir); err != nil {
					b.Fatalf("remove workdir: %v", err)
				}
			}
		})
	}
}

type benchmarkStreamingObjectSource struct {
	body []byte
}

func (s benchmarkStreamingObjectSource) OpenObject(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.body)), nil
}

func (s benchmarkStreamingObjectSource) ReadObject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

func benchmarkExportBundle(format string, itemCount int) ExportBundle {
	bundle := ExportBundle{
		Format:      format,
		Version:     "v-bench",
		Categories:  []string{"person"},
		CategoryIDs: []int64{7},
		Items:       make([]ExportItem, 0, itemCount),
	}

	for idx := 0; idx < itemCount; idx++ {
		item := ExportItem{
			ItemID:        int64(idx + 1),
			ObjectKey:     "train/" + strconv.Itoa(idx) + ".jpg",
			OutputName:    strconv.Itoa(idx) + ".jpg",
			LabelFileName: strconv.Itoa(idx) + ".txt",
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
					XCenter:    0.5,
					YCenter:    0.5,
					Width:      0.2,
					Height:     0.2,
				},
			},
		}
		bundle.Items = append(bundle.Items, item)
	}
	bundle.TotalBoxes = len(bundle.Items)
	return bundle
}
