package jobs

// transitions defines the only legal edges in the job lifecycle graph.
// Explicitly keeping this table small prevents accidental status drift.
var transitions = map[string]map[string]bool{
	StatusQueued: {
		StatusRunning:  true,
		StatusCanceled: true,
	},
	StatusRunning: {
		StatusSucceeded:           true,
		StatusSucceededWithErrors: true,
		StatusFailed:              true,
		StatusCanceled:            true,
		StatusRetryWaiting:        true,
	},
	StatusRetryWaiting: {
		StatusQueued: true,
	},
}

// CanTransition validates whether a status change is allowed.
// A no-op transition (from == to) is accepted for idempotent updates.
func CanTransition(from, to string) error {
	if from == to {
		return nil
	}
	allowed, ok := transitions[from]
	if !ok || !allowed[to] {
		return newConflictError("invalid transition: %s -> %s", from, to)
	}
	return nil
}
