package jobs

import "testing"

func TestTransitionToSucceededWithErrors(t *testing.T) {
	if err := CanTransition(StatusRunning, StatusSucceededWithErrors); err != nil {
		t.Fatalf("expected transition allowed, got %v", err)
	}
}

func TestTransitionFromFailedToRunningRejected(t *testing.T) {
	if err := CanTransition(StatusFailed, StatusRunning); err == nil {
		t.Fatal("expected transition to be rejected")
	}
}
