package versioning

import (
	"fmt"
	"math"
)

type Annotation struct {
	ItemID     int64   `json:"item_id"`
	CategoryID int64   `json:"category_id"`
	BBoxX      float64 `json:"bbox_x"`
	BBoxY      float64 `json:"bbox_y"`
	BBoxW      float64 `json:"bbox_w"`
	BBoxH      float64 `json:"bbox_h"`
}

type Change struct {
	ItemID     int64       `json:"item_id"`
	CategoryID int64       `json:"category_id"`
	Before     *Annotation `json:"before,omitempty"`
	After      *Annotation `json:"after,omitempty"`
	IOU        float64     `json:"iou,omitempty"`
}

type DiffStats struct {
	AddedCount      int     `json:"added_count"`
	RemovedCount    int     `json:"removed_count"`
	UpdatedCount    int     `json:"updated_count"`
	TotalBoxDelta   int     `json:"total_box_delta"`
	AverageIOUDrift float64 `json:"average_iou_drift"`
}

type DiffResult struct {
	Adds               []Change  `json:"adds"`
	Removes            []Change  `json:"removes"`
	Updates            []Change  `json:"updates"`
	Stats              DiffStats `json:"stats"`
	CompatibilityScore float64   `json:"compatibility_score"`
}

type Service struct {
	repo Repository
}

func NewService() *Service {
	return NewServiceWithRepository(nil)
}

func NewServiceWithRepository(repo Repository) *Service {
	return &Service{repo: repo}
}

func groupKey(itemID, categoryID int64) string {
	return fmt.Sprintf("%d:%d", itemID, categoryID)
}

func (s *Service) DiffSnapshots(before, after []Annotation, iouThreshold float64) DiffResult {
	if iouThreshold <= 0 {
		iouThreshold = 0.5
	}

	beforeGroups := make(map[string][]Annotation)
	afterGroups := make(map[string][]Annotation)
	for _, a := range before {
		beforeGroups[groupKey(a.ItemID, a.CategoryID)] = append(beforeGroups[groupKey(a.ItemID, a.CategoryID)], a)
	}
	for _, a := range after {
		afterGroups[groupKey(a.ItemID, a.CategoryID)] = append(afterGroups[groupKey(a.ItemID, a.CategoryID)], a)
	}

	out := DiffResult{
		Adds:    []Change{},
		Removes: []Change{},
		Updates: []Change{},
	}

	var iouSum float64
	keys := make(map[string]struct{}, len(beforeGroups)+len(afterGroups))
	for k := range beforeGroups {
		keys[k] = struct{}{}
	}
	for k := range afterGroups {
		keys[k] = struct{}{}
	}

	for k := range keys {
		beforeItems := beforeGroups[k]
		afterItems := afterGroups[k]
		matchedBefore := make([]bool, len(beforeItems))
		matchedAfter := make([]bool, len(afterItems))

		for bi, b := range beforeItems {
			for ai, a := range afterItems {
				if matchedAfter[ai] || !sameBBox(b, a) {
					continue
				}
				matchedBefore[bi] = true
				matchedAfter[ai] = true
				break
			}
		}

		for bi, b := range beforeItems {
			if matchedBefore[bi] {
				continue
			}

			bestIdx := -1
			bestIOU := 0.0
			for ai, a := range afterItems {
				if matchedAfter[ai] {
					continue
				}
				iou := bboxIOU(b, a)
				if iou > bestIOU {
					bestIOU = iou
					bestIdx = ai
				}
			}

			if bestIdx >= 0 && bestIOU >= iouThreshold {
				copyB := b
				copyA := afterItems[bestIdx]
				out.Updates = append(out.Updates, Change{ItemID: copyA.ItemID, CategoryID: copyA.CategoryID, Before: &copyB, After: &copyA, IOU: bestIOU})
				iouSum += bestIOU
				matchedBefore[bi] = true
				matchedAfter[bestIdx] = true
				continue
			}

			copyB := b
			out.Removes = append(out.Removes, Change{ItemID: b.ItemID, CategoryID: b.CategoryID, Before: &copyB})
			matchedBefore[bi] = true
		}

		for ai, a := range afterItems {
			if matchedAfter[ai] {
				continue
			}
			copyA := a
			out.Adds = append(out.Adds, Change{ItemID: a.ItemID, CategoryID: a.CategoryID, After: &copyA})
			matchedAfter[ai] = true
		}
	}

	out.Stats = DiffStats{
		AddedCount:    len(out.Adds),
		RemovedCount:  len(out.Removes),
		UpdatedCount:  len(out.Updates),
		TotalBoxDelta: len(out.Adds) - len(out.Removes),
	}
	if len(out.Updates) > 0 {
		out.Stats.AverageIOUDrift = iouSum / float64(len(out.Updates))
	}
	out.CompatibilityScore = compatibilityScore(len(before), len(after), len(out.Adds), len(out.Removes), out.Updates)

	return out
}

func (s *Service) DiffBySnapshotIDs(beforeSnapshotID, afterSnapshotID int64, iouThreshold float64) (DiffResult, error) {
	if s.repo == nil {
		return DiffResult{}, fmt.Errorf("annotation repository is not configured")
	}
	before, err := s.repo.ListEffectiveAnnotations(beforeSnapshotID)
	if err != nil {
		return DiffResult{}, err
	}
	after, err := s.repo.ListEffectiveAnnotations(afterSnapshotID)
	if err != nil {
		return DiffResult{}, err
	}
	return s.DiffSnapshots(before, after, iouThreshold), nil
}

func compatibilityScore(beforeCount, afterCount, addedCount, removedCount int, updates []Change) float64 {
	baseline := maxInt(beforeCount, afterCount, 1)
	exactMatches := baseline - addedCount - removedCount - len(updates)
	weightedSimilarity := float64(exactMatches)
	for _, update := range updates {
		weightedSimilarity += update.IOU
	}
	return clamp01(weightedSimilarity / float64(baseline))
}

func maxInt(values ...int) int {
	max := values[0]
	for _, value := range values[1:] {
		if value > max {
			max = value
		}
	}
	return max
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func sameBBox(a, b Annotation) bool {
	return a.BBoxX == b.BBoxX && a.BBoxY == b.BBoxY && a.BBoxW == b.BBoxW && a.BBoxH == b.BBoxH
}

func bboxIOU(a, b Annotation) float64 {
	ax2 := a.BBoxX + a.BBoxW
	ay2 := a.BBoxY + a.BBoxH
	bx2 := b.BBoxX + b.BBoxW
	by2 := b.BBoxY + b.BBoxH

	interW := math.Max(0, math.Min(ax2, bx2)-math.Max(a.BBoxX, b.BBoxX))
	interH := math.Max(0, math.Min(ay2, by2)-math.Max(a.BBoxY, b.BBoxY))
	inter := interW * interH
	if inter == 0 {
		return 0
	}

	aArea := a.BBoxW * a.BBoxH
	bArea := b.BBoxW * b.BBoxH
	denom := aArea + bArea - inter
	if denom <= 0 {
		return 0
	}
	return inter / denom
}
