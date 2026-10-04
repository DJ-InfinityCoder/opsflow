# OpsFlow Known Limitations

This document outlines the current technical boundaries, deliberate architectural trade-offs, and operational limitations in the existing implementation.

---

## 1. Authentication is Development-Only
- **No External IdP / OAuth**: The application currently uses `/auth/dev-login` and `/auth/demo-users` to generate HS256 JWT tokens for local development. It does not integrate with an enterprise identity provider (e.g. Okta, Azure AD, or Google Workspace).
- **Disabled in Production**: Dev login endpoints return HTTP 404 when `APP_ENV=production`. Real-world production deployment requires plugging in an OIDC or SAML provider to issue validated tokens.

## 2. Polling Instead of Server-Sent Events (SSE) / WebSockets
- **Interval Polling**: Real-time state updates (such as staleness detection every 10s and notification badges every 30s) are implemented using TanStack Query polling on intervals and window focus.
- **No Persistent Push Channel**: There is no live WebSocket or SSE connection from Postgres `LISTEN / NOTIFY` to the browser. Under high user concurrency, interval polling creates periodic read load on the database pool.

## 3. Asynchronous Deliveries are Stubs
- **No SMTP / Email Transport**: Worker jobs that trigger notifications write to the internal `notifications` table but do not dispatch emails via an external SMTP server or transactional provider (e.g. SendGrid, Postmark).
- **Webhooks are Logged**: The P1 incident webhook handler logs structured events to `slog` rather than making outbound HTTP POST requests to third-party endpoints (e.g. PagerDuty, Slack).

## 4. Wall-Clock SLA Schedules (No Business Hours or Calendars)
- **Continuous Wall-Clock Calculation**: SLA deadlines (`due_at`) are computed strictly as continuous wall-clock time from ticket creation (e.g. P1 = 1h, P2 = 4h, P3 = 24h, P4 = 72h).
- **No Team Schedules**: The engine does not account for business hours (e.g. 9am–5pm), regional public holidays, weekends, or on-call rotation schedules.

## 5. Audit Partitioning & Retention Policies Not Implemented
- **Append-Only Accumulation**: The `item_events` table enforces an append-only guarantee via a PostgreSQL trigger (`prevent_item_events_modification`), but does not have automated time-based table partitioning or archival jobs.
- **Unbounded Growth**: High-throughput installations will see `item_events` grow unbounded without periodic vacuuming and partition detachment strategies.

## 6. Full-Text Search Uses Basic Lexical Matching
- **Postgres tsvector Only**: Full-text search relies on Postgres `websearch_to_tsquery('english', ...)` against a stored `tsvector` column indexed via GIN.
- **No Advanced Relevance Ranking**: Search results are sorted by standard operational sorting (`priority ASC, updated_at DESC, id ASC`) rather than `ts_rank` or fuzzy typo-tolerance.

## 7. In-Memory Rate Limiting is Process-Local
- **Process Memory Buckets**: The HTTP rate limiter (`newIPRateLimiter`) uses an in-memory token bucket keyed by IP address.
- **Not Shared Across Replicas**: If multiple instances of the Go API run behind a load balancer, rate limits are not shared across processes and reset upon container restart. A shared Redis instance is required for distributed rate limiting.

## 8. Integration Tests Depend on Network Access to Supabase
- **Real Database Requirement**: In accordance with the non-negotiable engineering rules, integration tests run against a real PostgreSQL database (via `TEST_DATABASE_URL` falling back to `DATABASE_URL`) by dynamically provisioning isolated schemas (`test_<random>`).
- **Network Dependency**: Tests cannot run in completely air-gapped environments without network reachability to the Supabase pooler. Running multiple concurrent test suites against the same free-tier instance can temporarily saturate available connections.

## 9. Supabase Free-Tier Resource & Connection Constraints
- **Connection Caps**: Supabase free-tier PostgreSQL projects limit concurrent client connections (typically ~60 connections across all poolers and direct ports).
- **Connection Pools**: To prevent connection exhaustion, OpsFlow bounds `pgxpool.MaxConns` to 10 for the API, 4 for the worker, and limits test runners to 15 connections max. Scaling to multiple API/worker replicas requires upgrading to a Supabase Compute Add-on or self-hosted PgBouncer.
