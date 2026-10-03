
## Supabase Table Exposure

The Supabase auto-generated REST API (PostgREST) must not expose the OpsFlow tables. Every table migration enables row-level security and defines no policies, so `anon` and `authenticated` roles cannot read or mutate rows through the Supabase anon key. Keep these tables out of PostgREST's exposed schemas and route application data access through the Go API. The Go database connection uses the privileged Supabase `postgres` role, which bypasses row-level security.
