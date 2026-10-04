export type UserRole = "lead" | "operator" | "reporter"

export interface User {
  id: string
  email: string
  name: string
  is_system_admin: boolean
}

export interface Membership {
  team_id: string
  team_name: string
  role: UserRole
}

export interface MeResponse {
  user: User
  memberships: Membership[]
}

export interface DemoUsersResponse {
  users: User[]
}

export interface DevLoginResponse {
  access_token: string
  token_type: string
  expires_at: string
}

export type ItemStatus =
  | "new"
  | "triaged"
  | "in_progress"
  | "pending_approval"
  | "resolved"
  | "closed"

export interface WorkItem {
  id: string
  team_id: string
  team_name?: string
  title: string
  description?: string
  status: ItemStatus
  priority: number
  assignee_id?: string | null
  assignee_name?: string | null
  created_by: string
  created_by_name?: string
  custom_fields?: Record<string, unknown>
  version: number
  due_at?: string | null
  created_at: string
  updated_at: string
  resolved_at?: string | null
  sla_state?: "ok" | "warning" | "breached" | string
  allowed_transitions?: string[]
  pending_approval?: Approval | null
}

export interface Approval {
  id: string
  item_id: string
  requested_by: string
  decided_by?: string | null
  decision?: "approved" | "rejected" | null
  reason?: string | null
  created_at: string
  decided_at?: string | null
}

export interface TeamFieldSchema {
  team_id: string
  field_key: string
  label: string
  type: string
  required: boolean
  options?: unknown
}

export interface TeamMember {
  user_id: string
  name: string
  email: string
  role: UserRole
}

export interface ViewCounts {
  assigned_to_me: number
  team_unassigned: number
  urgent: number
  waiting_approval: number
  all: number
}

export interface ItemListResult {
  items: WorkItem[]
  next_cursor?: string
}

export interface ItemEvent {
  id: string | number
  item_id: string
  actor_id: string
  actor_name?: string
  type: string
  field?: string | null
  old_value?: unknown
  new_value?: unknown
  reason?: string | null
  payload?: Record<string, unknown>
  created_at: string
}

export interface Comment {
  id: string
  item_id: string
  author_id: string
  author_name?: string
  body: string
  mentions?: string[]
  created_at: string
}

export interface Notification {
  id: string
  user_id: string
  item_id?: string
  type: string
  title?: string
  body?: string
  source_event_id?: number | null
  dedupe_key?: string | null
  data?: Record<string, unknown>
  read_at?: string | null
  created_at: string
}

export interface NotificationPage {
  notifications: Notification[]
  next_cursor?: string
  unread_count?: number
}

export interface AnalyticsSummary {
  team_id: string
  total_items: number
  open_items: number
  by_status: Record<string, number>
  by_priority: Record<string | number, number>
  aging_buckets: Record<string, number>
  sla_breached_count: number
  sla_warning_count: number
  mttr_seconds: number
}

export interface AnalyticsTeamMetric {
  team_id: string
  team_name: string
  sla_breached_count: number
}

export interface FeedEvent {
  id: number
  item_id: string
  item_title: string
  team_id: string
  actor_id: string
  actor_name: string
  type: string
  field?: string | null
  reason?: string | null
  created_at: string
}

export interface FeedPage {
  events: FeedEvent[]
  next_cursor?: string
}

export interface OutboxJob {
  id: number
  type: string
  dedupe_key: string
  payload: Record<string, unknown> | unknown
  status: string
  attempts: number
  max_attempts: number
  run_at: string
  locked_at?: string | null
  last_error?: string | null
  created_at: string
}

export interface AdminJobsPage {
  jobs: OutboxJob[]
  next_cursor?: number
}

