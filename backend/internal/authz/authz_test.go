package authz_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/authz"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/service"
	"opsflow/backend/internal/testutil"
)

const (
	reporterRank = 1
	operatorRank = 2
	leadRank     = 3
)

type actionCase struct {
	name        string
	action      authz.Action
	minimumRank int
	adminOnly   bool
}

type roleCase struct {
	name  string
	user  model.User
	rank  int
	admin bool
}

func TestAuthorizeRoleActionMatrix(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	teamA := createTeam(t, ctx, pool, "Authorization Team A")
	teamB := createTeam(t, ctx, pool, "Authorization Team B")
	reporter := createUser(t, ctx, pool, "reporter-authz@example.test", false)
	operator := createUser(t, ctx, pool, "operator-authz@example.test", false)
	lead := createUser(t, ctx, pool, "lead-authz@example.test", false)
	admin := createUser(t, ctx, pool, "admin-authz@example.test", true)
	nonMember := createUser(t, ctx, pool, "outsider-authz@example.test", false)

	addMember(t, ctx, pool, teamA, reporter.ID, string(authz.RoleReporter))
	addMember(t, ctx, pool, teamA, operator.ID, string(authz.RoleOperator))
	addMember(t, ctx, pool, teamA, lead.ID, string(authz.RoleLead))
	addMember(t, ctx, pool, teamB, reporter.ID, string(authz.RoleReporter))

	authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(pool))
	actions := []actionCase{
		{name: "item.view", action: authz.Action{Name: authz.ActionItemView}, minimumRank: reporterRank},
		{name: "item.create", action: authz.Action{Name: authz.ActionItemCreate}, minimumRank: reporterRank},
		{name: "item.edit", action: authz.Action{Name: authz.ActionItemEdit}, minimumRank: operatorRank},
		{name: "item.claim", action: authz.Action{Name: authz.ActionItemClaim}, minimumRank: operatorRank},
		{name: "item.assign", action: authz.Action{Name: authz.ActionItemAssign}, minimumRank: leadRank},
		{name: "item.transition", action: authz.Action{Name: authz.ActionItemTransition, TargetState: "in_progress"}, minimumRank: operatorRank},
		{name: "approval.decide", action: authz.Action{Name: authz.ActionApprovalDecide}, minimumRank: leadRank},
		{name: "comment.create", action: authz.Action{Name: authz.ActionCommentCreate}, minimumRank: reporterRank},
		{name: "analytics.view", action: authz.Action{Name: authz.ActionAnalyticsView}, minimumRank: leadRank},
		{name: "admin.jobs.view", action: authz.Action{Name: authz.ActionAdminJobsView}, adminOnly: true},
	}
	roles := []roleCase{
		{name: "reporter", user: reporter, rank: reporterRank},
		{name: "operator", user: operator, rank: operatorRank},
		{name: "lead", user: lead, rank: leadRank},
		{name: "system_admin", user: admin, admin: true},
	}

	for _, role := range roles {
		for _, action := range actions {
			t.Run(role.name+"/"+action.name, func(t *testing.T) {
				teamID := teamA
				if role.admin {
					teamID = "00000000-0000-4000-8000-000000000099"
				}
				item := authz.Item{TeamID: teamID, CreatedBy: nonMember.ID, ApprovalRequestedBy: nonMember.ID}
				err := authorizer.Authorize(ctx, role.user, action.action, item)
				allowed := role.admin || (!action.adminOnly && role.rank >= action.minimumRank)
				if allowed {
					if err != nil {
						t.Fatalf("expected authorization, got %v", err)
					}
					return
				}
				requireAppError(t, err, service.ErrForbidden)
			})
		}
	}

	t.Run("non-member is hidden", func(t *testing.T) {
		err := authorizer.Authorize(ctx, nonMember, authz.Action{Name: authz.ActionItemView}, authz.Item{TeamID: teamA})
		requireAppError(t, err, service.ErrNotFound)
	})

	selfApprovalCases := []struct {
		name string
		item authz.Item
	}{
		{name: "requester", item: authz.Item{TeamID: teamA, CreatedBy: reporter.ID, ApprovalRequestedBy: lead.ID}},
		{name: "creator", item: authz.Item{TeamID: teamA, CreatedBy: lead.ID, ApprovalRequestedBy: reporter.ID}},
	}
	for _, testCase := range selfApprovalCases {
		t.Run("self approval forbidden/"+testCase.name, func(t *testing.T) {
			err := authorizer.Authorize(ctx, lead, authz.Action{Name: authz.ActionApprovalDecide}, testCase.item)
			requireAppError(t, err, service.ErrForbidden)
		})
	}

	t.Run("list accessible teams", func(t *testing.T) {
		teamIDs, err := authorizer.ListAccessibleTeamIDs(ctx, reporter)
		if err != nil {
			t.Fatalf("list reporter teams: %v", err)
		}
		assertTeamIDs(t, teamIDs, []string{teamA, teamB})

		adminTeamIDs, err := authorizer.ListAccessibleTeamIDs(ctx, admin)
		if err != nil {
			t.Fatalf("list admin teams: %v", err)
		}
		assertTeamIDs(t, adminTeamIDs, []string{teamA, teamB})
	})
}

func createTeam(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ($1) RETURNING id::text`, name).Scan(&id); err != nil {
		t.Fatalf("create team %s: %v", name, err)
	}
	return id
}

func createUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string, isAdmin bool) model.User {
	t.Helper()
	var user model.User
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, name, is_system_admin)
		VALUES ($1, $2, $3)
		RETURNING id::text, email, name, is_system_admin
	`, email, fmt.Sprintf("Test %s", email), isAdmin).Scan(&user.ID, &user.Email, &user.Name, &user.IsSystemAdmin); err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return user
}

func addMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool, teamID, userID, role string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)`, teamID, userID, role); err != nil {
		t.Fatalf("add %s membership: %v", role, err)
	}
}

func requireAppError(t *testing.T, actual, expected error) {
	t.Helper()
	if !errors.Is(actual, expected) {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

func assertTeamIDs(t *testing.T, actual, expected []string) {
	t.Helper()
	sort.Strings(actual)
	sort.Strings(expected)
	if len(actual) != len(expected) {
		t.Fatalf("expected team IDs %v, got %v", expected, actual)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("expected team IDs %v, got %v", expected, actual)
		}
	}
}
