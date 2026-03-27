package jobs

import "fmt"

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

func CanTransition(from, to string) error {
	if from == to {
		return nil
	}
	allowed, ok := transitions[from]
	if !ok {
		return fmt.Errorf("unsupported from status %q", from)
	}
	if !allowed[to] {
		return fmt.Errorf("invalid transition: %s -> %s", from, to)
	}
	return nil
}
