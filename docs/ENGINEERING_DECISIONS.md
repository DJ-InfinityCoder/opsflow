# OpsFlow Engineering Decisions

This document captures the architectural decisions, non-negotiable design principles, trade-offs, and scaling paths for the OpsFlow application.

---

## 1. Postgres-Only for Queue, Search, and Locks

### Context
Building an operational work tracker requires atomic write consistency, reliable asynchronous job execution (notifications, scans), full-text search across titles/descriptions, and distributed concurrency control. Adding external infrastructure (Redis, RabbitMQ, Elasticsearch) introduces distributed failure modes, dual-write synchronization bugs, and higher operational overhead.

### Choice
We strictly rely on **Supabase Postgres** for all data storage, queueing, full-text search, and locking:
- **Queue**: Implemented via the `outbox_jobs` table using `SELECT ... FOR UPDATE SKIP LOCKED`.
- **Search**: Built with PostgreSQL `tsvector` generated columns (`title || ' ' || description`) indexed via GIN (`to_tsvector('english', ...)`).
- **Concurrency & Locking**: Managed via PostgreSQL transaction isolation and row locks.
- No Docker, Redis, or external brokers are permitted.

### Trade-off
Postgres connection ceilings (particularly on managed Supabase instances) and write contention on the outbox table bound raw throughput compared to dedicated message brokers like Kafka or specialized search clusters.

### What to Change at Scale
If outbox throughput exceeds several thousand jobs per second, partition `outbox_jobs` by status/date and move completed job archiving off the critical write path. Only migrate to an external broker (e.g. AWS SQS or Kafka) if write contention on Postgres begins degrading API latency, while keeping transactional outbox publishing in Postgres.

---

## 2. Optimistic Locking for Edits vs Atomic Conditional Update for Claims

### Context
Work items are edited collaboratively by distributed operators. If two operators edit a ticket at the same time, one must not silently overwrite the other. However, ticket claiming is a competitive race where multiple operators rush to pick up unassigned items.

### Choice
We divide write mutations into two distinct concurrency models:
1. **Edits (PATCH /items/:id)**: Use **optimistic concurrency control** via an integer `work_items.version` column. Clients must send `If-Match: <version>`. If the database version does not match, the transaction rejects the edit with HTTP `409 Conflict` (`version_conflict`) and returns the current server item. The frontend displays an interactive diff resolution dialog ("Keep mine" vs "Use theirs").
2. **Claims (POST /items/:id/claim)**: Use an **atomic conditional update**:
   ```sql
   UPDATE work_items
   SET assignee_id = $1, status = 'in_progress', version = version + 1, updated_at = now()
   WHERE id = $2 AND assignee_id IS NULL AND status IN ('new', 'triaged');
   ```
   If zero rows are affected, the API queries the current row and returns HTTP `409 Conflict` (`already_claimed`) with winner metadata. The frontend optimistically assigns and rolls back immediately on 409.

### Trade-off
Optimistic locking places the burden of conflict resolution on the client and requires version tracking across round trips. Conditional updates cannot support complex multi-item reservations.

### What to Change at Scale
Keep single-item conditional updates atomic. For multi-item batch claiming, introduce short-lived Redis/Postgres advisory reservation locks with automatic TTL expiration.

---

## 3. Idempotency Stored in the Same Transaction as the Effect

### Context
In distributed environments, network drops or timeouts frequently cause clients to retry requests. If a client retries after the server committed the mutation but before receiving the HTTP response, the action could duplicate (e.g. creating duplicate tickets, re-approving items).

### Choice
We require `Idempotency-Key` (UUID) headers on all critical POST requests (`/items`, `/claim`, `/transition`, `/approvals/:aid/decide`).
- Inside the **exact same database transaction** as the mutation:
  1. We insert `(user_id, idempotency_key)` into `idempotency_keys` with a SHA-256 hash of `method + path + body`.
  2. If a matching record already exists with the same hash and a stored response, we immediately commit/rollback and replay the stored HTTP status and body with header `Idempotent-Replay: true`.
  3. If the key exists with a different hash, we abort with HTTP `422 Unprocessable Entity` (`idempotency_key_reuse`).
  4. If the key exists without a stored response (concurrent in-flight duplicate), we abort with HTTP `409 Conflict` (`request_in_progress`).
  5. The response payload is stored in `idempotency_keys` **before** the transaction commits.

### Trade-off
Writing response payloads into the database adds write I/O and slightly enlarges transaction size. The idempotency table requires routine cleanup of keys older than 24 hours.

### What to Change at Scale
Offload keys older than 24 hours to cold storage or let PostgreSQL automated table partitioning drop expired daily partitions (`idempotency_keys_2026_10_04`).

---

## 4. Single Service-Layer Authorization with 404/403 Semantics

### Context
OpsFlow users belong to multiple teams with distinct roles (`lead`, `operator`, `reporter`) and a global `is_system_admin` flag. Authorization logic scattered across HTTP handlers, SQL queries, or frontend components inevitably leads to privilege escalation bugs and information leaks.

### Choice
All authorization is centralized in a single Go function: `authorizer.Authorize(ctx, user, action, item)`.
- **404 for Non-Members**: If a user is not a member of the team owning the work item, the service returns HTTP `404 Not Found`. This prevents unauthorized users from determining whether an incident or item ID exists.
- **403 for Insufficient Role**: If a user is a member of the team but lacks the role required for the action (e.g. a `reporter` attempting to claim or resolve an item), the service returns HTTP `403 Forbidden`.
- **UI Hiding is Cosmetic**: Frontend button disabling or menu omission is purely for UX; the Go API never trusts client-asserted permissions.

### Trade-off
Every service mutation and read must load the user's team membership, adding a lookup query if not cached.

### What to Change at Scale
Cache user team memberships in memory (or a low-latency Redis cluster) with strict cache-invalidation triggers fired when team memberships change.

---

## 5. Transactional Outbox with At-Least-Once Delivery and Idempotent Consumers

### Context
Mutations frequently trigger secondary actions (e.g. sending notifications, triggering webhooks, SLA scans). Making external API or network calls inside a database transaction risks transaction timeouts and two-phase commit failures.

### Choice
We use the **Transactional Outbox Pattern**:
1. In the same database transaction that updates the work item, we insert an `outbox_jobs` row with a deterministic `dedupe_key` (e.g. `notify_assignment:{event_id}`).
2. An asynchronous worker claims pending jobs using `SELECT ... FOR UPDATE SKIP LOCKED WHERE status = 'pending' AND run_at <= now()`.
3. Failed jobs retry with exponential backoff and jitter up to `max_attempts` (default: 5), after which they transition to `failed` (dead-letter queue).
4. Unacknowledged jobs locked longer than 5 minutes are reclaimed.
5. All worker consumers are strictly idempotent:
   - Notification insertion uses `ON CONFLICT (user_id, dedupe_key) DO NOTHING`.

### Trade-off
Delivery guarantee is **at-least-once**. Consumers must handle potential duplicate deliveries gracefully.

### What to Change at Scale
Add dedicated Prometheus metrics for queue lag, dead-letter rates, and worker pool saturation. Separate high-frequency jobs (notifications) from long-running jobs (webhooks/scans) into distinct outbox queues.

---

## 6. Append-Only Audit Table with Trigger vs Full Event Sourcing

### Context
Compliance and operational transparency require a tamper-proof record of every change made to a work item (status transitions, priority changes, reassignment reasons).

### Choice
We use an **append-only `item_events` table**:
- Every item mutation records an event in the same transaction.
- A database-level trigger (`prevent_item_events_modification`) strictly raises an exception on any `UPDATE` or `DELETE` statement.
- The `work_items` table remains the canonical state store for direct query performance and index optimization, rather than replaying full event sourcing on every read.

### Trade-off
We do not get full event sourcing capabilities (e.g. arbitrary time-travel projection or retroactive event schema transforms).

### What to Change at Scale
Partition `item_events` by month or team ID to keep active working sets in memory. Archive older event partitions to cold object storage (S3/GCS) with retention locks.

---

## Supabase Table Exposure and RLS Deny-All

Every application table migration explicitly executes:
```sql
ALTER TABLE work_items ENABLE ROW LEVEL SECURITY;
```
No permissive RLS policies are created.

### Rationale:
- **Zero Direct PostgREST Access**: Supabase exposes PostgREST by default on anonymous and authenticated HTTP keys. By enabling RLS without policies, all public PostgREST queries evaluate to empty results (deny-all posture).
- **Go API is the Sole Data Path**: The Go API connects using the privileged Postgres connection string (`DATABASE_URL`), which bypasses RLS and enforces the canonical Go `Authorize()` layer. This guarantees that business rules, single-transaction invariants, and audit triggers can never be bypassed by web clients.
