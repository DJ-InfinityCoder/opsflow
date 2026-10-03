# OpsFlow Engineering Guide

## Product
OpsFlow is an internal operations work-item tracker for incidents, payment investigations, approvals, and related operational work. Users may belong to multiple teams and have a per-team role: `lead`, `operator`, or `reporter`. A system-admin flag applies globally.

## Stack and Configuration
- Backend: Go REST API using chi, pgx, and `slog`.
- Frontend: Next.js App Router, TypeScript, and Tailwind. Use shadcn/ui for all UI components and TanStack Query for server state.
- Database: Supabase Postgres only. Configure the Supabase session pooler connection on port 5432 with `DATABASE_URL`; support optional `TEST_DATABASE_URL` for tests.
- Do not add Dockerfiles, docker-compose files, or local Docker database instructions.
- Keep database connection usage within Supabase limits. Make pgxpool `MaxConns` configurable, defaulting to 10 for the API and 4 for the worker. Tests must never open more than 20 connections.

## Non-Negotiable Engineering Rules
1. Enforce authorization in the Go service layer through one function, `Authorize(user, action, item)`. Return 404 for non-members to avoid leaking item existence and 403 for a member with the wrong role. UI hiding is cosmetic and is never authorization.
2. Run every mutation in one database transaction, in this order: authorize; versioned/locked update; insert `item_events` audit record; insert `outbox_jobs`; store the idempotency response; commit.
3. Use optimistic locking with `work_items.version` (`int`). Updates require `If-Match: <version>`. On mismatch, return 409 `version_conflict` with the current item.
4. Make claim atomic with `UPDATE ... WHERE assignee_id IS NULL` and check rows affected. A losing claimant receives 409 `already_claimed` with winner information.
5. Require `Idempotency-Key` on critical POST requests. The same key and body replay the stored response; the same key with a different body returns 422.
6. Implement the state machine in Go: `New -> Triaged -> In Progress -> Pending Approval -> Resolved -> Closed`; `Pending Approval -> In Progress` for rejection; `Resolved -> In Progress` for reopening. Illegal transitions return 422 `illegal_transition` with allowed next states.
7. Make `item_events` append-only. A database trigger must raise on UPDATE or DELETE.
8. Use a Postgres outbox table for asynchronous work. Workers claim jobs with `FOR UPDATE SKIP LOCKED`, retry with exponential backoff, and dead-letter after `max_attempts`. Consumers must be idempotent; delivery is at least once.
9. Use keyset/cursor pagination only; never use OFFSET. Smart views are server-side queries.
10. Use one error envelope everywhere: `{"error":{"code":"...","message":"...","request_id":"...","details":{}}}`.
11. Use structured JSON logging and include a request ID.
12. Tests must use a real Postgres database via `TEST_DATABASE_URL`, falling back to `DATABASE_URL`. Each test run creates its own throwaway schema (for example, `test_<random>`), applies migrations with that schema as `search_path`, and drops the schema in cleanup. Never run tests against the `public` schema, and never TRUNCATE or drop anything in it. Do not use database mocks.

## Code Style and Architecture
- Keep Go layering as handlers -> service -> repository.
- Keep files small. Handlers do transport concerns only; business logic belongs in services.
- Write table-driven tests.
- Preserve the rules in this document for all later work. If a change appears to require an exception, document the decision in `docs/ENGINEERING_DECISIONS.md` rather than silently weakening a rule.
