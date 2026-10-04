# OpsFlow Two-Browser Demonstration Guide

This guide walks through step-by-step instructions to demonstrate OpsFlow's non-negotiable engineering capabilities using two concurrent browser sessions.

---

## Prerequisites & Environment Setup

1. **Start the API Server & Worker**:
   ```bash
   cd backend
   go run ./cmd/api
   ```
   *(Ensure `WORKER_ENABLED=true` in `backend/.env` or run `make worker` in a second terminal).*

2. **Start the Frontend Application**:
   ```bash
   cd frontend
   npm run dev
   ```
   Open `http://localhost:3000`.

3. **Open Two Independent Browser Sessions**:
   - **Session A**: Standard Browser window -> Log in as **Marcus** (`marcus@opsflow.local`, Operator).
   - **Session B**: Incognito / Private window -> Log in as **Alicia** (`alicia@opsflow.local`, Lead & System Admin) or **Priya** (`priya@opsflow.local`, Operator).

---

## Scenario 1: Atomic Concurrent Claim

**Goal**: Demonstrate atomic race condition prevention where two operators attempt to claim the same unassigned item simultaneously.

### Steps:
1. In **Session A (Marcus)**, navigate to the Dashboard (`/`). Filter by **Team Unassigned** tab or locate an unassigned item (e.g. `Item #...`).
2. In **Session B (Priya)**, navigate to the Dashboard (`/`) and locate the exact same unassigned item.
3. Align both browser windows side-by-side.
4. Click the **Claim** button on the item in both windows as close to simultaneously as possible.

### Expected Result:
- **Winning Operator (Session A)**:
  - Button transitions immediately; optimistic update confirms.
  - Item status changes to `In Progress`, assigned to `Marcus`, version increments (`v -> v+1`).
  - Toast: `Item claimed successfully`.
- **Losing Operator (Session B)**:
  - Server returns HTTP `409 Conflict` with error code `already_claimed` and details containing the winner's ID.
  - The optimistic UI rolls back immediately.
  - Toast: `<Marcus> claimed this just now`.
  - The row automatically reconciles and updates to show `Marcus` as the assignee.
  - Exactly **one** audit event (`claimed`) is recorded in the database.

---

## Scenario 2: Stale Edit & Optimistic Locking Conflict (409)

**Goal**: Demonstrate optimistic concurrency control using `If-Match: <version>`, conflict diff resolution, and non-destructive staleness banners.

### Steps:
1. In **Session A (Marcus)**, click on an item to open `/items/[id]` (e.g., version `1`).
2. Marcus types changes into the **Title** and **Description** fields, but **does not click Save yet**.
3. In **Session B (Alicia)**, open the same item `/items/[id]`.
4. Alicia changes the priority to `P1 Critical` and clicks **Save Changes**.
   - Alicia's save succeeds, bumping the item to version `2`.
5. Observe **Session A (Marcus)**:
   - Within 10 seconds (or immediately upon focusing the window), the **Staleness Banner** appears at the top:
     *"This item has been updated to version 2 by another operator."*
   - Marcus's typed draft is **preserved** and not discarded.
6. Now in **Session A (Marcus)**, click **Save Changes** (which sends `If-Match: 1`).

### Expected Result:
- The server rejects the mutation with HTTP `409 Conflict` (`version_conflict`).
- The **Version Conflict Resolution Dialog** opens on Marcus's screen:
  - Displays a side-by-side comparison per modified field:
    - **Title**: *Your draft* vs *Server value*
    - **Priority**: *Your draft (P3)* vs *Server value (P1)*
  - Offers interactive choices: **Keep mine**, **Use theirs**, or **Cancel**.
- Select **Keep mine** for title and **Use theirs** for priority, then click **Resolve & Save**.
- The frontend re-submits the mutation using the current version `2` with `If-Match: 2`.
- The update succeeds and reconciles cleanly to version `3`.

---

## Scenario 3: Double-Click & Network Retry Idempotency

**Goal**: Demonstrate that duplicate clicks or network retries reuse the client-generated `Idempotency-Key` (UUID) without creating duplicate side effects or approvals.

### Steps:
1. In **Session A (Marcus)**, open an item in `In Progress` status and click **Request Approval**. Provide a reason and submit.
2. In **Session B (Alicia - Team Lead)**, navigate to `/items/[id]`.
   - The item is in `Pending Approval`.
   - The **Approve** and **Reject** buttons are active and visible (only for leads who did not request it).
3. Open browser DevTools Network tab and set throttling to **Slow 3G** (or rapidly double-click the **Approve** button).
4. Alicia clicks **Approve**, types the required approval reason, and rapidly clicks **Confirm Decision** multiple times.

### Expected Result:
- The submit button is immediately disabled upon first click (`disabled={pending}`).
- Both rapid clicks send the exact same `Idempotency-Key: <uuid>`.
- In the backend:
  - The first request executes inside a database transaction, records the approval, transitions the item to `Resolved`, stores the response in `idempotency_keys`, and commits.
  - The concurrent/duplicate request matches the stored hash in `idempotency_keys` and returns the cached response with header `Idempotent-Replay: true`.
- Exactly **one** approval decision record and **one** audit event are created.
- The item resolves cleanly without duplicate status changes.

---

## Scenario 4: Outbox Worker Interruption & Exactly-Once Delivery

**Goal**: Demonstrate at-least-once outbox processing with idempotent consumers when a background worker crashes or restarts mid-flight.

### Steps:
1. Open a terminal running the background worker:
   ```bash
   cd backend
   go run ./cmd/worker
   ```
2. In **Session A**, post a comment on an item mentioning another operator:
   ```text
   Hey @priya please review this payment incident right away.
   ```
   - This writes an audit event, a comment, and queues an outbox job (`notify_mention`) inside the same transaction.
3. Immediately press `Ctrl+C` in the worker terminal while the job is claiming or running.
4. Observe the database outbox queue in **Session B (Alicia - System Admin)** at `/admin/jobs`:
   - The job is visible in the outbox queue (`notify_mention`).
5. Restart the worker:
   ```bash
   go run ./cmd/worker
   ```
6. The worker recovers:
   - Claims pending or reclaimed uncompleted jobs using `SELECT ... FOR UPDATE SKIP LOCKED`.
   - Delivers the notification to Priya.
   - The notification handler executes with idempotent constraints (`ON CONFLICT (user_id, dedupe_key) DO NOTHING`).

### Expected Result:
- In Priya's notification popover (`/notifications`), exactly **one** mention notification appears.
- Priya does **not** receive duplicate notifications despite worker interruption and re-run.
- In `/admin/jobs`, the job status transitions to `completed`.
