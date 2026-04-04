package versioning

import "testing"

func BenchmarkDiffSnapshotsExactMatches(b *testing.B) {
	svc := NewService()
	before, after := buildBenchmarkAnnotations(2000, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = svc.DiffSnapshots(before, after, 0.5)
	}
}

func BenchmarkDiffSnapshotsShiftedMatches(b *testing.B) {
	svc := NewService()
	before, after := buildBenchmarkAnnotations(2000, 0.5)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = svc.DiffSnapshots(before, after, 0.5)
	}
}

func buildBenchmarkAnnotations(count int, shift float64) ([]Annotation, []Annotation) {
	before := make([]Annotation, 0, count)
	after := make([]Annotation, 0, count)
	for idx := range count {
		itemID := int64(idx/4 + 1)
		categoryID := int64(idx%3 + 1)
		x := float64((idx % 40) * 8)
		y := float64((idx / 40) * 8)
		before = append(before, Annotation{
			ItemID:     itemID,
			CategoryID: categoryID,
			BBoxX:      x,
			BBoxY:      y,
			BBoxW:      6,
			BBoxH:      6,
		})
		after = append(after, Annotation{
			ItemID:     itemID,
			CategoryID: categoryID,
			BBoxX:      x + shift,
			BBoxY:      y + shift,
			BBoxW:      6,
			BBoxH:      6,
		})
	}
	return before, after
}
