package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"opsflow/backend/internal/config"
	"opsflow/backend/internal/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var (
		scale = flag.String("scale", "small", "seed size: small, medium, large")
		yes   = flag.Bool("yes", false, "skip confirmation prompt")
	)
	flag.Parse()

	scaleName := strings.ToLower(strings.TrimSpace(*scale))
	if scaleName == "" {
		scaleName = "small"
	}
	if !isValidScale(scaleName) {
		return fmt.Errorf("scale must be one of: small, medium, large")
	}
	if !*yes {
		return errors.New("this will populate seed data; re-run with --yes to confirm")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBConnectTimeout)
	if err != nil {
		return err
	}
	defer pool.Close()

	seedSize := map[string]int{"small": 250, "medium": 1500, "large": 5000}[scaleName]
	creatorNames := []string{"Alicia", "Marcus", "Priya", "Noah", "Elena", "Jonas", "Nina", "Omar"}
	teamNames := []string{"Payments", "Risk Ops", "Platform", "Support", "Analytics"}
	statusOrder := []string{"new", "triaged", "in_progress", "pending_approval", "resolved", "closed"}
	priorityOrder := []int{1, 2, 3, 4}
	workLabels := []string{"payment", "incident", "approval", "search", "customer", "invoice", "fraud", "retry", "payout", "investigation"}

	if _, err := pool.Exec(ctx, `
		INSERT INTO users (email, name, is_system_admin)
		VALUES
			('alicia@opsflow.local', 'Alicia', true),
			('marcus@opsflow.local', 'Marcus', false),
			('priya@opsflow.local', 'Priya', false),
			('noah@opsflow.local', 'Noah', false),
			('elena@opsflow.local', 'Elena', false),
			('jonas@opsflow.local', 'Jonas', false),
			('nina@opsflow.local', 'Nina', false),
			('omar@opsflow.local', 'Omar', false)
		ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, is_system_admin = EXCLUDED.is_system_admin;
	`); err != nil {
		return fmt.Errorf("upsert demo users: %w", err)
	}

	for _, name := range teamNames {
		if _, err := pool.Exec(ctx, `
			INSERT INTO teams (name)
			VALUES ($1)
			ON CONFLICT (name) DO NOTHING;
		`, name); err != nil {
			return fmt.Errorf("upsert team %q: %w", name, err)
		}
	}

	teamRows, err := pool.Query(ctx, `SELECT id, name FROM teams ORDER BY name`)
	if err != nil {
		return fmt.Errorf("load teams: %w", err)
	}
	defer teamRows.Close()

	teamIDs := map[string]string{}
	for teamRows.Next() {
		var id, name string
		if err := teamRows.Scan(&id, &name); err != nil {
			return fmt.Errorf("scan team: %w", err)
		}
		teamIDs[name] = id
	}
	if err := teamRows.Err(); err != nil {
		return fmt.Errorf("iterate teams: %w", err)
	}

	userRows, err := pool.Query(ctx, `SELECT id, email FROM users ORDER BY email`)
	if err != nil {
		return fmt.Errorf("load users: %w", err)
	}
	defer userRows.Close()
	userIDs := map[string]string{}
	for userRows.Next() {
		var id, email string
		if err := userRows.Scan(&id, &email); err != nil {
			return fmt.Errorf("scan user: %w", err)
		}
		userIDs[email] = id
	}
	if err := userRows.Err(); err != nil {
		return fmt.Errorf("iterate users: %w", err)
	}

	for _, teamName := range teamNames {
		teamID := teamIDs[teamName]
		for _, email := range []string{"alicia@opsflow.local", "marcus@opsflow.local", "priya@opsflow.local", "noah@opsflow.local", "elena@opsflow.local", "jonas@opsflow.local", "nina@opsflow.local", "omar@opsflow.local"} {
			userID := userIDs[email]
			if userID == "" {
				continue
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO team_members (team_id, user_id, role)
				VALUES ($1, $2, $3)
				ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role;
			`, teamID, userID, roleForTeam(teamName, email)); err != nil {
				return fmt.Errorf("upsert team member %s/%s: %w", teamName, email, err)
			}
		}
	}

	batchSize := 250
	rows := make([]string, 0, batchSize)
	args := make([]any, 0, batchSize*14)
	argIndex := 1
	for i := 0; i < seedSize; i++ {
		teamName := teamNames[i%len(teamNames)]
		teamID := teamIDs[teamName]
		createdBy := userIDs[creatorNames[(i+1)%len(creatorNames)]+"@opsflow.local"]
		if createdBy == "" {
			createdBy = userIDs["alicia@opsflow.local"]
		}
		assigneeEmail := ""
		if i%4 != 0 {
			assigneeEmail = []string{"marcus@opsflow.local", "priya@opsflow.local", "noah@opsflow.local", "elena@opsflow.local", "jonas@opsflow.local", "nina@opsflow.local"}[i%6]
		}
		assigneeID := interface{}(nil)
		if assigneeEmail != "" {
			assigneeID = userIDs[assigneeEmail]
		}
		status := statusOrder[i%len(statusOrder)]
		priority := priorityOrder[i%len(priorityOrder)]
		label := workLabels[(i+1)%len(workLabels)]
		labelTitle := strings.ToUpper(label[:1]) + label[1:]
		title := fmt.Sprintf("%s request #%d", labelTitle, i+1)
		description := fmt.Sprintf("Follow up on %s for the %s team. Investigations cover payment investigation, vendor coordination, uptime risk, and customer communications for item %d.", label, teamName, i+1)
		createdAt := time.Now().Add(-time.Duration((i%180)*24) * time.Hour).Add(-time.Duration((i % 12)) * time.Hour)
		updatedAt := createdAt.Add(time.Duration((i%10)+1) * time.Minute)
		customFields, _ := json.Marshal(map[string]any{
			"customer_id": fmt.Sprintf("CUST-%04d", i%5000),
			"owner":       teamName,
			"severity":    fmt.Sprintf("sev-%d", (i%4)+1),
		})
		dueAt := createdAt.Add(72 * time.Hour)
		if i%7 == 0 {
			dueAt = createdAt.Add(24 * time.Hour)
		}
		if i%9 == 0 {
			status = "closed"
			dueAt = createdAt.Add(12 * time.Hour)
		}

		placeholders := make([]string, 0, 14)
		for j := 0; j < 13; j++ {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
			argIndex++
		}
		rows = append(rows, "("+strings.Join(placeholders, ",")+")")
		args = append(args,
			teamID,
			title,
			description,
			status,
			priority,
			createdBy,
			assigneeID,
			customFields,
			dueAt,
			1,
			createdAt,
			updatedAt,
			nil,
		)
		if len(rows) >= batchSize || i == seedSize-1 {
			query := `WITH inserted_items AS (
				INSERT INTO work_items (team_id, title, description, status, priority, created_by, assignee_id, custom_fields, due_at, version, created_at, updated_at, resolved_at)
				VALUES ` + strings.Join(rows, ",") + `
				RETURNING id, status, created_by
			)
			INSERT INTO approvals (item_id, requested_by)
			SELECT id, created_by FROM inserted_items WHERE status = 'pending_approval'`
			if _, err := pool.Exec(ctx, query, args...); err != nil {
				return fmt.Errorf("insert work item batch and pending approvals: %w", err)
			}
			rows = rows[:0]
			args = args[:0]
			argIndex = 1
		}
	}
	fmt.Printf("Seeded %d items across %d teams and %d users.\n", seedSize, len(teamNames), len(userIDs))
	return nil
}

func isValidScale(s string) bool {
	_, ok := map[string]bool{"small": true, "medium": true, "large": true}[s]
	return ok
}

func roleForTeam(teamName, email string) string {
	roles := map[string]string{
		"Payments":  "lead",
		"Risk Ops":  "operator",
		"Platform":  "operator",
		"Support":   "reporter",
		"Analytics": "lead",
	}
	if email == "alicia@opsflow.local" {
		return "lead"
	}
	if email == "marcus@opsflow.local" && teamName == "Payments" {
		return "lead"
	}
	return roles[teamName]
}
