# OpsFlow

OpsFlow is an internal operations work-item tracker engineered for incident management, payment investigations, approvals, and asynchronous operational workflows.

The system is built as a Go REST API with pgx and chi paired with a Next.js (App Router) TypeScript frontend using shadcn/ui and TanStack Query. All persistence, queueing, locking, and search rely strictly on Supabase Postgres.

---

## Architecture

```mermaid
flowchart TD
    subgraph Client["Client (Browser)"]
        UI["Next.js App Router (React 19 / TypeScript)"]
        RQ["TanStack Query (Cache & State)"]
        UI <--> RQ
    end

    subgraph Backend["Backend Services (Go)"]
        Router["Chi Router & Middleware (CORS, Rate Limit, Auth, ReqID)"]
        Authz["Canonical Authorizer (authz.Authorize)"]
        Service["Service Layer (Items, Statemachine, Comments, Approvals)"]
        Worker["Outbox Worker (SKIP LOCKED, Retries, Backoff)"]
    end

    subgraph Database["Supabase Postgres (Port 5432 Session Pooler)"]
        Items["work_items (Versioned, Optimistic Locks)"]
        Events["item_events (Append-only via Trigger)"]
        Outbox["outbox_jobs (SKIP LOCKED Queue)"]
        Idemp["idempotency_keys (Request Hash & Replay)"]
        Notifs["notifications (Idempotent Delivery)"]
    end

    UI -->|HTTP / JSON + Bearer Token| Router
    Router --> Service
    Service --> Authz
    Service -->|Single Transaction Mutation| Items
    Service -->|Single Transaction Mutation| Events
    Service -->|Single Transaction Mutation| Outbox
    Service -->|Single Transaction Mutation| Idemp
    Worker -->|Claim Jobs: FOR UPDATE SKIP LOCKED| Outbox
    Worker -->|Insert Notification / Log Webhook| Notifs
```

---

## Prerequisites

1. **Go**: Version `1.23` or newer (declared in `backend/go.mod`).
2. **Node.js**: Version `18.18+` or `20+` with `npm`.
3. **Supabase Project**: A Postgres database instance. Ensure you have the session pooler connection string on port `5432`.
4. **Make**: Standard build tool (`make`) for running automation targets (or execute corresponding `go` commands).

---

## Quick Start Instructions

### 1. Backend Setup & Configuration

Create the backend environment file from the template:

```bash
# In Linux / macOS:
cp backend/.env.example backend/.env

# In Windows PowerShell:
Copy-Item backend/.env.example backend/.env
```

Open `backend/.env` and configure:
```dotenv
DATABASE_URL=postgresql://postgres.[project-ref]:[password]@aws-0-[region].pooler.supabase.com:5432/postgres?sslmode=require
JWT_SECRET=4f9b8c2e6d1a5f7b3e0c8a2d4e6f9a1b5c3d7e8f0a2b4c6d8e1f3a5b7c9d0e2f
PORT=8080
WORKER_ENABLED=true
APP_ENV=development
DB_MAX_CONNS=10
DB_CONNECT_TIMEOUT=5s
```

### 2. Database Migrations

From the `backend/` directory, apply all database migrations:

```bash
cd backend
make migrate
```
*(Runs `go run ./cmd/migrate up` applying all schema tables, triggers, indexes, and RLS deny-all policies).*

### 3. Seed Database

Populate teams, field schemas, users, and realistic work items:

```bash
# Seed standard dataset (default: small, 250 items):
make seed ARGS="--scale=small --yes"

# Or seed large dataset (5,000 items, recommended for benchmarks):
make seed ARGS="--scale=large --yes"
```

### 4. Run the Backend API & Worker

Run the API server with embedded background worker (`WORKER_ENABLED=true`):

```bash
cd backend
make run
```

*Alternatively, disable `WORKER_ENABLED` in `.env` and run the worker independently in another terminal:*
```bash
cd backend
make worker
```

### 5. Frontend Setup & Launch

In a separate terminal, install dependencies and start the Next.js development server:

```bash
cd frontend
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000) in your browser.

---

## Demo Logins

When running in development (`APP_ENV=development`), the login page provides a one-click demo user switcher populated from the database:

| Name | Email | Role | Permissions |
|---|---|---|---|
| **Alicia** | `alicia@opsflow.local` | **Lead & System Admin** | Full administrative controls, decision approvals, analytics, failed jobs inspector. |
| **Marcus** | `marcus@opsflow.local` | **Operator** | Claims items, edits fields, transitions tickets, adds comments. |
| **Priya** | `priya@opsflow.local` | **Operator** | Claims items, handles payments triage, receives assignment & mention alerts. |
| **Noah** | `noah@opsflow.local` | **Reporter** | Creates tickets, views public items (restricted from claiming or resolving). |

---

## Running Tests

### Backend Test Suite
Runs table-driven tests, state machine tests, concurrency tests, and idempotency tests against a real Supabase Postgres instance using throwaway schemas (`test_<random>`):

```bash
cd backend
make test
```
*(Runs `go test ./... -race -count=1` keeping connections strictly under 20).*

### Frontend Test Suite
Runs the Vitest + Testing Library test suite (optimistic cache rollback, idempotency key rotation, and version conflict resolution):

```bash
cd frontend
npm test
```

To run lint checks:
```bash
cd frontend
npm run lint
```

To build production bundle:
```bash
cd frontend
npm run build
```

---

## Supabase Troubleshooting

1. **Session Pooler (Port 5432) vs Direct vs Transaction Pooler (Port 6543)**:
   - **Always use Port 5432 (Session Pooler)**: Resolves over IPv4 and supports prepared statements, session locks, and advisory locks.
   - **Do NOT use Direct Connections**: Supabase direct connections (`db.[project-ref].supabase.co:5432`) are IPv6-only and will fail to resolve on most residential or local dev networks without explicit IPv6 configurations.
   - **Do NOT use Port 6543 (Transaction Pooler)**: PgBouncer transaction mode prohibits prepared statements (`prepared_statement_cache`) and causes errors with `pgx`.
2. **SSL Mode**:
   - Supabase connection strings require `sslmode=require` query parameter. Omitting it will result in connection rejection.
3. **Connection Limits**:
   - Supabase free-tier database instances have strict connection maximums (~60 connections).
   - OpsFlow bounds connection consumption via `DB_MAX_CONNS=10` for API, 4 for the outbox worker, and limits integration tests to a shared pool of 15 connections max. Never configure pool sizes exceeding 20 connections in local development.