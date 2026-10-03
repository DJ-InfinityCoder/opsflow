package statemachine

import (
	"errors"
	"fmt"
)

var (
	ErrIllegalTransition = errors.New("illegal state transition")
	ErrPriorityRequired  = errors.New("transition to triaged requires priority to be set")
	ErrAssigneeRequired  = errors.New("transition to in_progress requires an assignee")
	ErrApprovalRequired  = errors.New("transition to resolved requires an approved decision from the approval flow")
)

const (
	StateNew             = "new"
	StateTriaged         = "triaged"
	StateInProgress      = "in_progress"
	StatePendingApproval = "pending_approval"
	StateResolved        = "resolved"
	StateClosed          = "closed"
)

var transitions = map[string][]string{
	StateNew:             {StateTriaged},
	StateTriaged:         {StateInProgress},
	StateInProgress:      {StatePendingApproval},
	StatePendingApproval: {StateInProgress, StateResolved},
	StateResolved:        {StateInProgress, StateClosed},
	StateClosed:          {},
}

// Allowed returns a copy of allowed target states for the current status.
func Allowed(status string) []string {
	allowed := transitions[status]
	return append([]string(nil), allowed...)
}

// CanTransition checks if the transition graph permits moving from -> to.
func CanTransition(from, to string) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// ItemSnapshot captures the fields needed to evaluate transition preconditions.
type ItemSnapshot struct {
	Status           string
	Priority         int16
	AssigneeID       *string
	ApprovalApproved bool
}

// CheckPreconditions verifies whether the transition from -> to is allowed
// and satisfies all state machine preconditions.
func CheckPreconditions(from, to string, item ItemSnapshot) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrIllegalTransition, from, to)
	}

	switch to {
	case StateTriaged:
		if item.Priority < 1 || item.Priority > 4 {
			return ErrPriorityRequired
		}
	case StateInProgress:
		if item.AssigneeID == nil || *item.AssigneeID == "" {
			return ErrAssigneeRequired
		}
	case StateResolved:
		if from == StatePendingApproval && !item.ApprovalApproved {
			return ErrApprovalRequired
		}
	}
	return nil
}
