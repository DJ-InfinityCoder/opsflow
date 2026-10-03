package statemachine_test

import (
	"errors"
	"testing"

	"opsflow/backend/internal/statemachine"
)

func TestTransitionsTable(t *testing.T) {
	tests := []struct {
		status  string
		allowed []string
	}{
		{status: statemachine.StateNew, allowed: []string{statemachine.StateTriaged}},
		{status: statemachine.StateTriaged, allowed: []string{statemachine.StateInProgress}},
		{status: statemachine.StateInProgress, allowed: []string{statemachine.StatePendingApproval}},
		{status: statemachine.StatePendingApproval, allowed: []string{statemachine.StateInProgress, statemachine.StateResolved}},
		{status: statemachine.StateResolved, allowed: []string{statemachine.StateInProgress, statemachine.StateClosed}},
		{status: statemachine.StateClosed, allowed: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			allowed := statemachine.Allowed(tt.status)
			if len(allowed) != len(tt.allowed) {
				t.Fatalf("for %s expected %v, got %v", tt.status, tt.allowed, allowed)
			}
			for i, s := range tt.allowed {
				if allowed[i] != s {
					t.Errorf("at index %d expected %s, got %s", i, s, allowed[i])
				}
				if !statemachine.CanTransition(tt.status, s) {
					t.Errorf("CanTransition(%s, %s) should be true", tt.status, s)
				}
			}
		})
	}
}

func TestCheckPreconditions(t *testing.T) {
	validAssignee := "user-123"

	tests := []struct {
		name        string
		from        string
		to          string
		item        statemachine.ItemSnapshot
		expectedErr error
	}{
		{
			name:        "new to triaged without priority fails",
			from:        statemachine.StateNew,
			to:          statemachine.StateTriaged,
			item:        statemachine.ItemSnapshot{Priority: 0},
			expectedErr: statemachine.ErrPriorityRequired,
		},
		{
			name:        "new to triaged with priority succeeds",
			from:        statemachine.StateNew,
			to:          statemachine.StateTriaged,
			item:        statemachine.ItemSnapshot{Priority: 2},
			expectedErr: nil,
		},
		{
			name:        "triaged to in_progress without assignee fails",
			from:        statemachine.StateTriaged,
			to:          statemachine.StateInProgress,
			item:        statemachine.ItemSnapshot{AssigneeID: nil},
			expectedErr: statemachine.ErrAssigneeRequired,
		},
		{
			name:        "triaged to in_progress with assignee succeeds",
			from:        statemachine.StateTriaged,
			to:          statemachine.StateInProgress,
			item:        statemachine.ItemSnapshot{AssigneeID: &validAssignee},
			expectedErr: nil,
		},
		{
			name:        "in_progress to pending_approval succeeds",
			from:        statemachine.StateInProgress,
			to:          statemachine.StatePendingApproval,
			item:        statemachine.ItemSnapshot{},
			expectedErr: nil,
		},
		{
			name:        "pending_approval to resolved without approval decision fails",
			from:        statemachine.StatePendingApproval,
			to:          statemachine.StateResolved,
			item:        statemachine.ItemSnapshot{ApprovalApproved: false},
			expectedErr: statemachine.ErrApprovalRequired,
		},
		{
			name:        "pending_approval to resolved with approved decision succeeds",
			from:        statemachine.StatePendingApproval,
			to:          statemachine.StateResolved,
			item:        statemachine.ItemSnapshot{ApprovalApproved: true},
			expectedErr: nil,
		},
		{
			name:        "pending_approval to in_progress (rejection) with assignee succeeds",
			from:        statemachine.StatePendingApproval,
			to:          statemachine.StateInProgress,
			item:        statemachine.ItemSnapshot{AssigneeID: &validAssignee},
			expectedErr: nil,
		},
		{
			name:        "resolved to in_progress (reopen) with assignee succeeds",
			from:        statemachine.StateResolved,
			to:          statemachine.StateInProgress,
			item:        statemachine.ItemSnapshot{AssigneeID: &validAssignee},
			expectedErr: nil,
		},
		{
			name:        "resolved to closed succeeds",
			from:        statemachine.StateResolved,
			to:          statemachine.StateClosed,
			item:        statemachine.ItemSnapshot{},
			expectedErr: nil,
		},
		{
			name:        "new directly to in_progress is illegal",
			from:        statemachine.StateNew,
			to:          statemachine.StateInProgress,
			item:        statemachine.ItemSnapshot{AssigneeID: &validAssignee, Priority: 1},
			expectedErr: statemachine.ErrIllegalTransition,
		},
		{
			name:        "closed to in_progress is illegal",
			from:        statemachine.StateClosed,
			to:          statemachine.StateInProgress,
			item:        statemachine.ItemSnapshot{AssigneeID: &validAssignee},
			expectedErr: statemachine.ErrIllegalTransition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := statemachine.CheckPreconditions(tt.from, tt.to, tt.item)
			if tt.expectedErr == nil {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if !errors.Is(err, tt.expectedErr) {
				t.Fatalf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}
