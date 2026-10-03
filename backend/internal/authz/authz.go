package authz

import (
	"context"
	"strings"

	"opsflow/backend/internal/apperr"
	"opsflow/backend/internal/model"
)

type ActionName string

const (
	ActionItemView       ActionName = "item.view"
	ActionItemCreate     ActionName = "item.create"
	ActionItemEdit       ActionName = "item.edit"
	ActionItemClaim      ActionName = "item.claim"
	ActionItemAssign     ActionName = "item.assign"
	ActionItemTransition ActionName = "item.transition"
	ActionApprovalDecide ActionName = "approval.decide"
	ActionCommentCreate  ActionName = "comment.create"
	ActionAnalyticsView  ActionName = "analytics.view"
	ActionAdminJobsView  ActionName = "admin.jobs.view"
)

type Action struct {
	Name        ActionName
	TargetState string
}

type Item struct {
	TeamID              string
	CreatedBy           string
	ApprovalRequestedBy string
}

type Role string

const (
	RoleLead     Role = "lead"
	RoleOperator Role = "operator"
	RoleReporter Role = "reporter"
)

type MembershipStore interface {
	RoleForTeam(ctx context.Context, userID, teamID string) (Role, bool, error)
	ListTeamIDs(ctx context.Context, userID string) ([]string, error)
	ListAllTeamIDs(ctx context.Context) ([]string, error)
}

type Authorizer struct {
	memberships MembershipStore
}

func NewAuthorizer(memberships MembershipStore) *Authorizer {
	return &Authorizer{memberships: memberships}
}

func (a *Authorizer) Authorize(ctx context.Context, user model.User, action Action, item Item) error {
	minimumRole, globalOnly, err := requiredRole(action)
	if err != nil {
		return err
	}
	if action.Name == ActionItemTransition && strings.TrimSpace(action.TargetState) == "" {
		return apperr.New(apperr.KindValidation, "transition target state is required", nil)
	}

	if action.Name == ActionApprovalDecide && (sameID(user.ID, item.CreatedBy) || sameID(user.ID, item.ApprovalRequestedBy)) {
		return apperr.ErrForbidden
	}
	if user.IsSystemAdmin {
		return nil
	}
	if globalOnly {
		return apperr.ErrForbidden
	}
	if strings.TrimSpace(item.TeamID) == "" {
		return apperr.ErrNotFound
	}

	role, isMember, err := a.memberships.RoleForTeam(ctx, user.ID, item.TeamID)
	if err != nil {
		return err
	}
	if !isMember {
		return apperr.ErrNotFound
	}
	if !roleSatisfies(role, minimumRole) {
		return apperr.ErrForbidden
	}
	return nil
}

func (a *Authorizer) ListAccessibleTeamIDs(ctx context.Context, user model.User) ([]string, error) {
	if user.IsSystemAdmin {
		return a.memberships.ListAllTeamIDs(ctx)
	}
	return a.memberships.ListTeamIDs(ctx, user.ID)
}

func requiredRole(action Action) (Role, bool, error) {
	switch action.Name {
	case ActionItemView, ActionItemCreate, ActionCommentCreate:
		return RoleReporter, false, nil
	case ActionItemEdit, ActionItemClaim:
		return RoleOperator, false, nil
	case ActionItemTransition:
		if strings.EqualFold(action.TargetState, "resolved") {
			return RoleLead, false, nil
		}
		return RoleOperator, false, nil
	case ActionItemAssign, ActionApprovalDecide, ActionAnalyticsView:
		return RoleLead, false, nil
	case ActionAdminJobsView:
		return "", true, nil
	default:
		return "", false, apperr.ErrForbidden
	}
}

func roleSatisfies(actual, required Role) bool {
	rank := map[Role]int{
		RoleReporter: 1,
		RoleOperator: 2,
		RoleLead:     3,
	}
	return rank[actual] >= rank[required] && rank[actual] != 0
}

func sameID(left, right string) bool {
	return left != "" && right != "" && strings.EqualFold(left, right)
}
