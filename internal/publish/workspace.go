package publish

func buildWorkspace(batch Batch) Workspace {
	items := make([]WorkspaceItem, 0, len(batch.Items))
	for _, item := range batch.Items {
		itemFeedback := make([]Feedback, 0)
		for _, entry := range batch.Feedback {
			if entry.Scope == FeedbackScopeItem && entry.PublishBatchItemID == item.ID {
				itemFeedback = append(itemFeedback, entry)
			}
		}

		items = append(items, WorkspaceItem{
			ItemID:      item.ID,
			CandidateID: item.CandidateID,
			TaskID:      item.TaskID,
			Overlay:     nestedPayload(item.ItemPayload, "overlay"),
			Diff:        nestedPayload(item.ItemPayload, "diff"),
			Context:     nestedPayload(item.ItemPayload, "context"),
			Feedback:    itemFeedback,
		})
	}

	history := make([]TimelineEntry, 0, 2)
	if batch.ReviewApprovedBy != "" && batch.ReviewApprovedAt != nil {
		history = append(history, TimelineEntry{
			Stage:  FeedbackStageReview,
			Actor:  batch.ReviewApprovedBy,
			Action: ReviewDecisionApprove,
			At:     *batch.ReviewApprovedAt,
		})
	}
	if batch.OwnerDecidedBy != "" && batch.OwnerDecidedAt != nil {
		history = append(history, TimelineEntry{
			Stage:  FeedbackStageOwner,
			Actor:  batch.OwnerDecidedBy,
			Action: batch.Status,
			At:     *batch.OwnerDecidedAt,
		})
	}

	return Workspace{
		Batch:   batch,
		Items:   items,
		History: history,
	}
}

func nestedPayload(payload map[string]any, key string) map[string]any {
	value, ok := payload[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return cloneJSONMap(value)
}
