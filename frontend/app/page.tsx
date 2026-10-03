"use client"

import * as React from "react"
import Link from "next/link"
import { useSearchParams, useRouter, usePathname } from "next/navigation"
import {
  useInfiniteQuery,
  useQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query"
import { useVirtualizer } from "@tanstack/react-virtual"
import { useAuth } from "@/providers/auth-provider"
import { apiFetch, ApiError } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { WorkItem, ItemListResult, ViewCounts } from "@/types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Skeleton } from "@/components/ui/skeleton"
import { toast } from "sonner"
import {
  Search,
  RefreshCw,
  AlertTriangle,
  Clock,
  CheckCircle2,
  XCircle,
  FilterX,
  User as UserIcon,
  Loader2,
  Building2,
} from "lucide-react"

function formatRelativeTime(dateString: string): string {
  try {
    const date = new Date(dateString)
    const now = new Date()
    const diffMs = now.getTime() - date.getTime()
    if (diffMs < 0) return "just now"
    const diffSec = Math.floor(diffMs / 1000)
    if (diffSec < 60) return "just now"
    const diffMin = Math.floor(diffSec / 60)
    if (diffMin < 60) return `${diffMin}m ago`
    const diffHr = Math.floor(diffMin / 60)
    if (diffHr < 24) return `${diffHr}h ago`
    const diffDays = Math.floor(diffHr / 24)
    if (diffDays < 7) return `${diffDays}d ago`
    return date.toLocaleDateString(undefined, { month: "short", day: "numeric" })
  } catch {
    return dateString
  }
}

function getPriorityBadge(priority: number) {
  switch (priority) {
    case 1:
      return (
        <Badge variant="outline" className="border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400 font-bold text-[11px] px-1.5 py-0">
          P1 Critical
        </Badge>
      )
    case 2:
      return (
        <Badge variant="outline" className="border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400 font-semibold text-[11px] px-1.5 py-0">
          P2 High
        </Badge>
      )
    case 3:
      return (
        <Badge variant="outline" className="border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-400 font-medium text-[11px] px-1.5 py-0">
          P3 Medium
        </Badge>
      )
    default:
      return (
        <Badge variant="outline" className="border-zinc-500/30 bg-zinc-500/10 text-zinc-600 dark:text-zinc-400 text-[11px] px-1.5 py-0">
          P4 Low
        </Badge>
      )
  }
}

function getStatusBadge(status: string) {
  switch (status) {
    case "new":
      return (
        <Badge variant="outline" className="border-sky-500/30 bg-sky-500/10 text-sky-600 dark:text-sky-400 text-[11px] capitalize px-2 py-0.5">
          New
        </Badge>
      )
    case "triaged":
      return (
        <Badge variant="outline" className="border-purple-500/30 bg-purple-500/10 text-purple-600 dark:text-purple-400 text-[11px] capitalize px-2 py-0.5">
          Triaged
        </Badge>
      )
    case "in_progress":
      return (
        <Badge variant="outline" className="border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400 text-[11px] capitalize px-2 py-0.5">
          In Progress
        </Badge>
      )
    case "pending_approval":
      return (
        <Badge variant="outline" className="border-orange-500/30 bg-orange-500/10 text-orange-600 dark:text-orange-400 text-[11px] capitalize px-2 py-0.5">
          Pending Approval
        </Badge>
      )
    case "resolved":
      return (
        <Badge variant="outline" className="border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 text-[11px] capitalize px-2 py-0.5">
          Resolved
        </Badge>
      )
    case "closed":
      return (
        <Badge variant="outline" className="border-zinc-500/30 bg-zinc-500/10 text-zinc-600 dark:text-zinc-400 text-[11px] capitalize px-2 py-0.5">
          Closed
        </Badge>
      )
    default:
      return (
        <Badge variant="outline" className="text-[11px] capitalize px-2 py-0.5">
          {status}
        </Badge>
      )
  }
}

function getSlaBadge(slaState?: string) {
  if (!slaState) return null
  switch (slaState) {
    case "ok":
      return (
        <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 border border-emerald-500/20 px-2 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
          <CheckCircle2 className="size-3" />
          SLA OK
        </span>
      )
    case "warning":
      return (
        <span className="inline-flex items-center gap-1 rounded-full bg-amber-500/10 border border-amber-500/20 px-2 py-0.5 text-[10px] font-semibold text-amber-600 dark:text-amber-400">
          <Clock className="size-3" />
          SLA Warning
        </span>
      )
    case "breached":
      return (
        <span className="inline-flex items-center gap-1 rounded-full bg-red-500/10 border border-red-500/20 px-2 py-0.5 text-[10px] font-bold text-red-600 dark:text-red-400">
          <AlertTriangle className="size-3" />
          SLA Breached
        </span>
      )
    default:
      return null
  }
}

function DashboardContent() {
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const queryClient = useQueryClient()
  const { user, memberships } = useAuth()

  // Read filter state from URL search params
  const currentView = searchParams.get("view") || "all"
  const currentTeam = searchParams.get("team") || ""
  const currentStatus = searchParams.get("status") || ""
  const currentPriority = searchParams.get("priority") || ""
  const currentAssignee = searchParams.get("assignee") || ""
  const currentQ = searchParams.get("q") || ""

  // Local state for debounced search box
  const [searchInput, setSearchInput] = React.useState(currentQ)

  // Keep searchInput in sync if URL param changes externally
  React.useEffect(() => {
    setSearchInput(currentQ)
  }, [currentQ])

  const updateFilter = React.useCallback(
    (key: string, value: string | null) => {
      const params = new URLSearchParams(searchParams.toString())
      if (value && value !== "all" && value !== "") {
        params.set(key, value)
      } else {
        params.delete(key)
      }
      router.replace(`${pathname}?${params.toString()}`)
    },
    [searchParams, router, pathname]
  )

  // Debounce search input changes by 300ms
  React.useEffect(() => {
    const handler = setTimeout(() => {
      if (searchInput !== currentQ) {
        updateFilter("q", searchInput.trim() ? searchInput.trim() : null)
      }
    }, 300)
    return () => clearTimeout(handler)
  }, [searchInput, currentQ, updateFilter])

  const clearAllFilters = () => {
    router.replace(pathname)
    setSearchInput("")
  }

  // 1. Live counts from GET /views/counts
  const { data: counts, isLoading: isLoadingCounts } = useQuery<ViewCounts>({
    queryKey: queryKeys.views.counts(),
    queryFn: () => apiFetch<ViewCounts>("/views/counts"),
    refetchInterval: 30000,
    refetchOnWindowFocus: true,
  })

  // 2. Infinite query for items list
  const filterParams = React.useMemo(() => {
    const p: Record<string, string> = {}
    if (currentView) p.view = currentView
    if (currentTeam) p.team = currentTeam
    if (currentStatus) p.status = currentStatus
    if (currentPriority) p.priority = currentPriority
    if (currentAssignee) p.assignee = currentAssignee
    if (currentQ) p.q = currentQ
    return p
  }, [currentView, currentTeam, currentStatus, currentPriority, currentAssignee, currentQ])

  const activeQueryKey = React.useMemo(
    () => queryKeys.items.list(filterParams),
    [filterParams]
  )

  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
  } = useInfiniteQuery<ItemListResult>({
    queryKey: activeQueryKey,
    queryFn: async ({ pageParam }) => {
      const sp = new URLSearchParams(filterParams)
      if (pageParam && typeof pageParam === "string") {
        sp.set("cursor", pageParam)
      }
      sp.set("limit", "50")
      return apiFetch<ItemListResult>(`/items?${sp.toString()}`)
    },
    initialPageParam: "",
    getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
    refetchInterval: 30000,
    refetchOnWindowFocus: true,
  })

  // Flatten items across loaded pages
  const allItems = React.useMemo(
    () => data?.pages.flatMap((page) => page.items) ?? [],
    [data]
  )

  // Virtualizer container
  const parentRef = React.useRef<HTMLDivElement>(null)
  // TanStack Virtual returns functions that React Compiler cannot safely memoize.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: hasNextPage ? allItems.length + 1 : allItems.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 76,
    overscan: 6,
  })

  // Claim mutation with optimistic update
  const claimMutation = useMutation({
    mutationFn: async (item: WorkItem) => {
      const idempotencyKey = crypto.randomUUID()
      return apiFetch<WorkItem>(`/items/${item.id}/claim`, {
        method: "POST",
        body: {},
        idempotencyKey,
      })
    },
    onMutate: async (item: WorkItem) => {
      await queryClient.cancelQueries({ queryKey: activeQueryKey })
      await queryClient.cancelQueries({ queryKey: queryKeys.views.counts() })

      const previousData = queryClient.getQueryData<InfiniteData<ItemListResult>>(activeQueryKey)

      // Optimistically update item in cache
      queryClient.setQueryData<InfiniteData<ItemListResult>>(
        activeQueryKey,
        (old) => {
          if (!old) return old
          return {
            ...old,
            pages: old.pages.map((page) => ({
              ...page,
              items: page.items.map((row) =>
                row.id === item.id
                  ? {
                      ...row,
                      assignee_id: user?.id ?? "me",
                      assignee_name: user?.name ?? "Me",
                      status: "in_progress",
                      version: row.version + 1,
                    }
                  : row
              ),
            })),
          }
        }
      )

      return { previousData }
    },
    onError: (err, item, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(activeQueryKey, context.previousData)
      }

      if (err instanceof ApiError && (err.status === 409 || err.code === "already_claimed")) {
        const winner = err.details?.winner as { assignee_id?: string } | undefined
        const winnerId = winner?.assignee_id || "Another operator"
        toast.error(`${winnerId} claimed this just now`)

        const serverItem = err.details?.current_item as WorkItem | undefined
        if (serverItem) {
          queryClient.setQueryData<InfiniteData<ItemListResult>>(
            activeQueryKey,
            (old) => {
              if (!old) return old
              return {
                ...old,
                pages: old.pages.map((page) => ({
                  ...page,
                  items: page.items.map((row) => (row.id === serverItem.id ? serverItem : row)),
                })),
              }
            }
          )
        } else {
          queryClient.invalidateQueries({ queryKey: activeQueryKey })
        }
      } else {
        toast.error(err instanceof Error ? err.message : "Failed to claim item")
      }
    },
    onSuccess: (updatedItem) => {
      toast.success("Item claimed successfully")
      queryClient.setQueryData<InfiniteData<ItemListResult>>(
        activeQueryKey,
        (old) => {
          if (!old) return old
          return {
            ...old,
            pages: old.pages.map((page) => ({
              ...page,
              items: page.items.map((row) => (row.id === updatedItem.id ? updatedItem : row)),
            })),
          }
        }
      )
      queryClient.invalidateQueries({ queryKey: queryKeys.views.counts() })
    },
  })

  // Tabs configuration
  const tabs = [
    { id: "all", label: "All", count: counts?.all },
    { id: "assigned_to_me", label: "Assigned to Me", count: counts?.assigned_to_me },
    { id: "team_unassigned", label: "Team Unassigned", count: counts?.team_unassigned },
    { id: "urgent", label: "Urgent (P1/P2)", count: counts?.urgent },
    { id: "waiting_approval", label: "Waiting on Approval", count: counts?.waiting_approval },
  ]

  const hasActiveFilters =
    currentTeam !== "" ||
    currentStatus !== "" ||
    currentPriority !== "" ||
    currentAssignee !== "" ||
    currentQ !== ""

  return (
    <div className="flex h-full flex-col gap-4">
      {/* Header and View Tabs */}
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight text-foreground">
              Work Items
            </h1>
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={() => {
                refetch()
                queryClient.invalidateQueries({ queryKey: queryKeys.views.counts() })
              }}
              title="Refresh live list"
            >
              <RefreshCw className="size-3.5 text-muted-foreground" />
            </Button>
          </div>
        </div>

        {/* Tab Selector */}
        <div className="flex overflow-x-auto border-b pb-px scrollbar-none">
          <div className="flex gap-1">
            {tabs.map((tab) => {
              const isActive = currentView === tab.id
              return (
                <button
                  key={tab.id}
                  onClick={() => updateFilter("view", tab.id === "all" ? null : tab.id)}
                  className={`inline-flex items-center gap-2 border-b-2 px-3.5 py-2 text-xs font-semibold whitespace-nowrap transition-colors ${
                    isActive
                      ? "border-primary text-primary"
                      : "border-transparent text-muted-foreground hover:text-foreground hover:border-border"
                  }`}
                >
                  <span>{tab.label}</span>
                  <span
                    className={`rounded-full px-1.5 py-0.2 text-[10px] font-bold ${
                      isActive
                        ? "bg-primary/15 text-primary"
                        : "bg-muted text-muted-foreground"
                    }`}
                  >
                    {isLoadingCounts ? "-" : tab.count ?? 0}
                  </span>
                </button>
              )
            })}
          </div>
        </div>
      </div>

      {/* Filter Bar */}
      <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-card/60 p-2.5 shadow-2xs backdrop-blur-sm">
        {/* Search Input */}
        <div className="relative flex-1 min-w-[200px]">
          <Search className="pointer-events-none absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
          <Input
            placeholder="Search items by title or description..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="h-9 pl-8 text-xs bg-background/80"
          />
        </div>

        {/* Team Filter */}
        <div className="flex items-center">
          <select
            value={currentTeam}
            onChange={(e) => updateFilter("team", e.target.value)}
            aria-label="Filter by Team"
            className="h-9 rounded-md border border-input bg-background/80 px-2.5 py-1 text-xs shadow-2xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="">All Teams</option>
            {memberships.map((m) => (
              <option key={m.team_id} value={m.team_id}>
                {m.team_name}
              </option>
            ))}
          </select>
        </div>

        {/* Status Filter */}
        <div className="flex items-center">
          <select
            value={currentStatus}
            onChange={(e) => updateFilter("status", e.target.value)}
            aria-label="Filter by Status"
            className="h-9 rounded-md border border-input bg-background/80 px-2.5 py-1 text-xs shadow-2xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="">All Statuses</option>
            <option value="new">New</option>
            <option value="triaged">Triaged</option>
            <option value="in_progress">In Progress</option>
            <option value="pending_approval">Pending Approval</option>
            <option value="resolved">Resolved</option>
            <option value="closed">Closed</option>
          </select>
        </div>

        {/* Priority Filter */}
        <div className="flex items-center">
          <select
            value={currentPriority}
            onChange={(e) => updateFilter("priority", e.target.value)}
            aria-label="Filter by Priority"
            className="h-9 rounded-md border border-input bg-background/80 px-2.5 py-1 text-xs shadow-2xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="">All Priorities</option>
            <option value="1">P1 Critical</option>
            <option value="2">P2 High</option>
            <option value="3">P3 Medium</option>
            <option value="4">P4 Low</option>
          </select>
        </div>

        {/* Assignee Filter */}
        <div className="flex items-center">
          <select
            value={currentAssignee}
            onChange={(e) => updateFilter("assignee", e.target.value)}
            aria-label="Filter by Assignee"
            className="h-9 rounded-md border border-input bg-background/80 px-2.5 py-1 text-xs shadow-2xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="">All Assignees</option>
            <option value="me">Assigned to Me</option>
          </select>
        </div>

        {/* Clear Filters Button */}
        {hasActiveFilters && (
          <Button
            variant="ghost"
            size="sm"
            onClick={clearAllFilters}
            className="h-9 text-xs gap-1 text-muted-foreground hover:text-foreground"
          >
            <FilterX className="size-3.5" />
            <span>Reset</span>
          </Button>
        )}
      </div>

      {/* Main List Area */}
      <div className="flex-1 min-h-[450px] rounded-lg border bg-card shadow-2xs overflow-hidden flex flex-col">
        {isLoading ? (
          // Loading Skeleton
          <div className="p-4 space-y-3">
            {Array.from({ length: 7 }).map((_, i) => (
              <div
                key={i}
                className="flex items-center justify-between p-3 rounded-lg border bg-muted/20 animate-pulse"
              >
                <div className="flex items-center gap-3">
                  <Skeleton className="size-8 rounded-full" />
                  <div className="space-y-1.5">
                    <Skeleton className="h-4 w-64" />
                    <div className="flex items-center gap-2">
                      <Skeleton className="h-3 w-16" />
                      <Skeleton className="h-3 w-20" />
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <Skeleton className="h-6 w-16 rounded-md" />
                  <Skeleton className="h-6 w-14 rounded-md" />
                </div>
              </div>
            ))}
          </div>
        ) : isError ? (
          // Error State with Retry
          <div className="flex flex-1 flex-col items-center justify-center p-8 text-center">
            <XCircle className="size-10 text-destructive mb-3" />
            <h3 className="text-base font-semibold">Failed to load work items</h3>
            <p className="text-xs text-muted-foreground mt-1 max-w-md">
              {error instanceof Error ? error.message : "An unexpected network or server error occurred."}
            </p>
            <Button
              variant="outline"
              size="sm"
              onClick={() => refetch()}
              className="mt-4 gap-1.5"
            >
              <RefreshCw className="size-3.5" />
              <span>Retry Request</span>
            </Button>
          </div>
        ) : allItems.length === 0 ? (
          // Empty State
          <div className="flex flex-1 flex-col items-center justify-center p-8 text-center">
            <div className="flex size-12 items-center justify-center rounded-full bg-muted/60 text-muted-foreground mb-3">
              <Search className="size-5" />
            </div>
            <h3 className="text-base font-semibold">No work items found</h3>
            <p className="text-xs text-muted-foreground mt-1 max-w-sm">
              {hasActiveFilters
                ? "No work items match the selected filters. Try broadening your search or resetting filters."
                : "No work items have been recorded in this view yet."}
            </p>
            {hasActiveFilters && (
              <Button
                variant="outline"
                size="sm"
                onClick={clearAllFilters}
                className="mt-4 gap-1.5 text-xs"
              >
                <FilterX className="size-3.5" />
                <span>Clear All Filters</span>
              </Button>
            )}
          </div>
        ) : (
          // Virtualized List
          <div
            ref={parentRef}
            className="flex-1 overflow-y-auto divide-y divide-border/60"
          >
            <div
              style={{
                height: `${virtualizer.getTotalSize()}px`,
                width: "100%",
                position: "relative",
              }}
            >
              {virtualizer.getVirtualItems().map((virtualRow) => {
                const isLoaderRow = virtualRow.index >= allItems.length
                const item = allItems[virtualRow.index]

                if (isLoaderRow) {
                  if (hasNextPage && !isFetchingNextPage) {
                    fetchNextPage()
                  }
                  return (
                    <div
                      key="loader-row"
                      style={{
                        position: "absolute",
                        top: 0,
                        left: 0,
                        width: "100%",
                        height: `${virtualRow.size}px`,
                        transform: `translateY(${virtualRow.start}px)`,
                      }}
                      className="flex items-center justify-center p-3 text-xs text-muted-foreground gap-2"
                    >
                      <Loader2 className="size-4 animate-spin" />
                      <span>Loading more items...</span>
                    </div>
                  )
                }

                const canClaim =
                  !item.assignee_id &&
                  (item.status === "new" ||
                    item.status === "triaged" ||
                    item.status === "in_progress")

                const isClaiming = claimMutation.isPending && claimMutation.variables?.id === item.id

                return (
                  <div
                    key={item.id}
                    style={{
                      position: "absolute",
                      top: 0,
                      left: 0,
                      width: "100%",
                      transform: `translateY(${virtualRow.start}px)`,
                    }}
                    className="flex flex-col sm:flex-row sm:items-center justify-between p-3.5 px-4 gap-2.5 transition-colors hover:bg-muted/40"
                  >
                    {/* Item Information */}
                    <div className="flex items-start gap-3 min-w-0 flex-1">
                      {/* Priority Badge */}
                      <div className="shrink-0 pt-0.5">{getPriorityBadge(item.priority)}</div>

                      <div className="min-w-0 flex-1 space-y-1">
                        <div className="flex items-center gap-2">
                          <Link
                            href={`/items/${item.id}`}
                            className="font-semibold text-sm text-foreground truncate hover:text-primary hover:underline underline-offset-2 transition-colors"
                          >
                            {item.title}
                          </Link>
                        </div>

                        <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                          {item.team_name ? (
                            <span className="flex items-center gap-1 font-medium">
                              <Building2 className="size-3" />
                              {item.team_name}
                            </span>
                          ) : (
                            <span className="font-mono text-[10px]">
                              {item.team_id.slice(0, 8)}
                            </span>
                          )}
                          <span>•</span>
                          <span>Updated {formatRelativeTime(item.updated_at)}</span>
                          {item.due_at && (
                            <>
                              <span>•</span>
                              <span>Due {formatRelativeTime(item.due_at)}</span>
                            </>
                          )}
                        </div>
                      </div>
                    </div>

                    {/* Metadata & Actions */}
                    <div className="flex items-center justify-between sm:justify-end gap-2.5 shrink-0 pt-1 sm:pt-0">
                      {/* SLA Badge */}
                      {getSlaBadge(item.sla_state)}

                      {/* Status Badge */}
                      {getStatusBadge(item.status)}

                      {/* Assignee Avatar */}
                      <div className="flex items-center">
                        {item.assignee_id ? (
                          <div
                            className="flex items-center gap-1.5"
                            title={`Assigned to ${item.assignee_name || item.assignee_id}`}
                          >
                            <Avatar className="size-6 border">
                              <AvatarFallback className="text-[10px] font-bold bg-primary/10 text-primary">
                                {item.assignee_name
                                  ? item.assignee_name.slice(0, 2).toUpperCase()
                                  : "OP"}
                              </AvatarFallback>
                            </Avatar>
                          </div>
                        ) : (
                          <span className="text-[10px] text-muted-foreground italic px-1.5">
                            Unassigned
                          </span>
                        )}
                      </div>

                      {/* Inline Claim Button */}
                      {canClaim && (
                        <Button
                          size="xs"
                          variant="outline"
                          onClick={() => claimMutation.mutate(item)}
                          disabled={isClaiming}
                          className="h-7 text-xs font-semibold gap-1 border-primary/40 hover:bg-primary hover:text-primary-foreground"
                        >
                          {isClaiming ? (
                            <Loader2 className="size-3 animate-spin" />
                          ) : (
                            <UserIcon className="size-3" />
                          )}
                          <span>Claim</span>
                        </Button>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default function DashboardPage() {
  return (
    <React.Suspense
      fallback={
        <div className="space-y-4 p-4">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-96 w-full" />
        </div>
      }
    >
      <DashboardContent />
    </React.Suspense>
  )
}
