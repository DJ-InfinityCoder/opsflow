export interface ItemFilters {
  team_id?: string
  status?: string
  priority?: number
  assignee_id?: string
  view?: "my_work" | "triage" | "all"
  search?: string
  cursor?: string
  limit?: number
  sort?: string
  order?: "asc" | "desc"
}

export const queryKeys = {
  auth: {
    all: ["auth"] as const,
    me: () => [...queryKeys.auth.all, "me"] as const,
    demoUsers: () => [...queryKeys.auth.all, "demo-users"] as const,
  },
  views: {
    all: ["views"] as const,
    counts: () => [...queryKeys.views.all, "counts"] as const,
  },
  items: {
    all: ["items"] as const,
    lists: () => [...queryKeys.items.all, "list"] as const,
    list: (filters: ItemFilters = {}) => [...queryKeys.items.lists(), filters] as const,
    details: () => [...queryKeys.items.all, "detail"] as const,
    detail: (id: string) => [...queryKeys.items.details(), id] as const,
    events: (id: string) => [...queryKeys.items.detail(id), "events"] as const,
    comments: (id: string) => [...queryKeys.items.detail(id), "comments"] as const,
    counts: (filters: Record<string, unknown> = {}) => [...queryKeys.items.all, "counts", filters] as const,
  },
  notifications: {
    all: ["notifications"] as const,
    list: (unreadOnly = false) => [...queryKeys.notifications.all, "list", { unreadOnly }] as const,
    unreadCount: () => [...queryKeys.notifications.all, "unread-count"] as const,
  },
  teams: {
    all: ["teams"] as const,
    list: () => [...queryKeys.teams.all, "list"] as const,
    detail: (id: string) => [...queryKeys.teams.all, "detail", id] as const,
    members: (teamId: string) => [...queryKeys.teams.all, teamId, "members"] as const,
    schemas: (teamId: string) => [...queryKeys.teams.all, teamId, "schemas"] as const,
  },
  analytics: {
    all: ["analytics"] as const,
    teams: () => [...queryKeys.analytics.all, "teams"] as const,
    summary: (teamId?: string) => [...queryKeys.analytics.all, "summary", { teamId }] as const,
    workload: (teamId?: string) => [...queryKeys.analytics.all, "workload", { teamId }] as const,
    aging: (teamId?: string) => [...queryKeys.analytics.all, "aging", { teamId }] as const,
  },
  feed: {
    all: ["feed"] as const,
    list: (teamId?: string) => [...queryKeys.feed.all, "list", { teamId }] as const,
  },
  admin: {
    all: ["admin"] as const,
    jobs: (status = "failed") => [...queryKeys.admin.all, "jobs", { status }] as const,
  },
} as const
