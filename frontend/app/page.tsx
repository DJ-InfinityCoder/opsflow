"use client"

import * as React from "react"
import Link from "next/link"
import { useSearchParams, useRouter, usePathname } from "next/navigation"
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { useVirtualizer } from "@tanstack/react-virtual"
import { useAuth } from "@/providers/auth-provider"
import { apiFetch } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { ItemListResult, ViewCounts } from "@/types"
import { useClaimWorkItem } from "@/hooks/use-claim-work-item"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Skeleton } from "@/components/ui/skeleton"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
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
  ChevronDown,
  Check,
  CircleDot,
  SlidersHorizontal,
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

  const claimMutation = useClaimWorkItem(activeQueryKey, user)

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

  const STATUS_OPTIONS = [
    { value: "", label: "All Statuses" },
    { value: "new", label: "New", color: "bg-sky-500" },
    { value: "triaged", label: "Triaged", color: "bg-purple-500" },
    { value: "in_progress", label: "In Progress", color: "bg-amber-500" },
    { value: "pending_approval", label: "Pending Approval", color: "bg-orange-500" },
    { value: "resolved", label: "Resolved", color: "bg-emerald-500" },
    { value: "closed", label: "Closed", color: "bg-zinc-500" },
  ]
  const selectedStatus = STATUS_OPTIONS.find((s) => s.value === currentStatus)

  const PRIORITY_OPTIONS = [
    { value: "", label: "All Priorities" },
    { value: "1", label: "P1 Critical", color: "bg-red-500" },
    { value: "2", label: "P2 High", color: "bg-amber-500" },
    { value: "3", label: "P3 Medium", color: "bg-blue-500" },
    { value: "4", label: "P4 Low", color: "bg-zinc-500" },
  ]
  const selectedPriority = PRIORITY_OPTIONS.find((p) => p.value === currentPriority)
  const selectedTeam = memberships.find((m) => m.team_id === currentTeam)

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
              variant="outline"
              size="icon-xs"
              className="size-7 rounded-sm border border-border bg-background hover:bg-accent text-foreground shadow-none"
              onClick={() => {
                refetch()
                queryClient.invalidateQueries({ queryKey: queryKeys.views.counts() })
              }}
              title="Refresh live list"
            >
              <RefreshCw className="size-3.5" />
            </Button>
          </div>
        </div>

        {/* Tab Selector */}
        <div className="flex overflow-x-auto pb-1 scrollbar-none">
          <div className="inline-flex items-center gap-1 rounded-sm border border-border bg-muted/30 p-1">
            {tabs.map((tab) => {
              const isActive = currentView === tab.id
              return (
                <button
                  key={tab.id}
                  onClick={() => updateFilter("view", tab.id === "all" ? null : tab.id)}
                  className={`inline-flex items-center gap-2 rounded-sm px-3 py-1.5 text-xs transition-all ${
                    isActive
                      ? "bg-background text-foreground font-semibold border border-border shadow-none"
                      : "text-muted-foreground hover:text-foreground hover:bg-background/40 border border-transparent font-medium"
                  }`}
                >
                  <span>{tab.label}</span>
                  <span
                    className={`rounded-sm px-1.5 py-0.5 text-[10px] font-mono ${
                      isActive
                        ? "bg-primary/10 text-primary font-bold"
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

      {/* Filter Bar with shadcn DropdownMenu */}
      <div className="flex flex-wrap items-center gap-2 rounded-sm border border-border bg-card p-2.5 shadow-none">
        {/* Search Input */}
        <div className="relative flex-1 min-w-[200px]">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
          <Input
            placeholder="Search items by title or description..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="h-8 pl-8 text-xs bg-background border border-border rounded-sm shadow-none"
          />
        </div>

        {/* Team Filter Dropdown */}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="outline"
                size="sm"
                className="h-8 gap-1.5 text-xs font-normal border border-border bg-background hover:bg-accent justify-between rounded-sm shadow-none"
              />
            }
          >
            <Building2 className="size-3.5 text-muted-foreground" />
            <span className="truncate max-w-[120px]">
              {selectedTeam ? selectedTeam.team_name : "All Teams"}
            </span>
            <ChevronDown className="size-3 opacity-50 ml-0.5" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-52 border border-border bg-popover shadow-none rounded-md">
            <DropdownMenuLabel>Filter by Team</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() => updateFilter("team", "")}
              className="text-xs justify-between cursor-pointer"
            >
              <span>All Teams</span>
              {!currentTeam && <Check className="size-3.5 text-primary" />}
            </DropdownMenuItem>
            {memberships.map((m) => (
              <DropdownMenuItem
                key={m.team_id}
                onClick={() => updateFilter("team", m.team_id)}
                className="text-xs justify-between cursor-pointer"
              >
                <span className="truncate">{m.team_name}</span>
                {currentTeam === m.team_id && <Check className="size-3.5 text-primary" />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        {/* Status Filter Dropdown */}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="outline"
                size="sm"
                className="h-8 gap-1.5 text-xs font-normal border border-border bg-background hover:bg-accent justify-between rounded-sm shadow-none"
              />
            }
          >
            <CircleDot className="size-3.5 text-muted-foreground" />
            <span>{selectedStatus?.label || "All Statuses"}</span>
            <ChevronDown className="size-3 opacity-50 ml-0.5" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-48 border border-border bg-popover shadow-none rounded-md">
            <DropdownMenuLabel>Filter by Status</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {STATUS_OPTIONS.map((opt) => (
              <DropdownMenuItem
                key={opt.value}
                onClick={() => updateFilter("status", opt.value)}
                className="text-xs justify-between cursor-pointer"
              >
                <div className="flex items-center gap-2">
                  {opt.color && <span className={`size-2 rounded-full ${opt.color}`} />}
                  <span>{opt.label}</span>
                </div>
                {(currentStatus || "") === opt.value && <Check className="size-3.5 text-primary" />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        {/* Priority Filter Dropdown */}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="outline"
                size="sm"
                className="h-8 gap-1.5 text-xs font-normal border border-border bg-background hover:bg-accent justify-between rounded-sm shadow-none"
              />
            }
          >
            <SlidersHorizontal className="size-3.5 text-muted-foreground" />
            <span>{selectedPriority?.label || "All Priorities"}</span>
            <ChevronDown className="size-3 opacity-50 ml-0.5" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-44 border border-border bg-popover shadow-none rounded-md">
            <DropdownMenuLabel>Filter by Priority</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {PRIORITY_OPTIONS.map((opt) => (
              <DropdownMenuItem
                key={opt.value}
                onClick={() => updateFilter("priority", opt.value)}
                className="text-xs justify-between cursor-pointer"
              >
                <div className="flex items-center gap-2">
                  {opt.color && <span className={`size-2 rounded-full ${opt.color}`} />}
                  <span>{opt.label}</span>
                </div>
                {(currentPriority || "") === opt.value && <Check className="size-3.5 text-primary" />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        {/* Assignee Filter Dropdown */}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="outline"
                size="sm"
                className="h-8 gap-1.5 text-xs font-normal border border-border bg-background hover:bg-accent justify-between rounded-sm shadow-none"
              />
            }
          >
            <UserIcon className="size-3.5 text-muted-foreground" />
            <span>{currentAssignee === "me" ? "Assigned to Me" : "All Assignees"}</span>
            <ChevronDown className="size-3 opacity-50 ml-0.5" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-44 border border-border bg-popover shadow-none rounded-md">
            <DropdownMenuLabel>Filter by Assignee</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={() => updateFilter("assignee", "")}
              className="text-xs justify-between cursor-pointer"
            >
              <span>All Assignees</span>
              {!currentAssignee && <Check className="size-3.5 text-primary" />}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => updateFilter("assignee", "me")}
              className="text-xs justify-between cursor-pointer"
            >
              <span>Assigned to Me</span>
              {currentAssignee === "me" && <Check className="size-3.5 text-primary" />}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        {/* Clear Filters Button */}
        {hasActiveFilters && (
          <Button
            variant="outline"
            size="sm"
            onClick={clearAllFilters}
            className="h-8 text-xs gap-1 border border-dashed border-border bg-background hover:bg-accent text-muted-foreground hover:text-foreground rounded-sm shadow-none"
          >
            <FilterX className="size-3.5" />
            <span>Reset</span>
          </Button>
        )}
      </div>

      {/* Main List Area */}
      <div className="flex-1 min-h-[480px] rounded-sm border border-border bg-card shadow-none overflow-hidden flex flex-col">
        {/* Table Column Header */}
        <div className="flex items-center justify-between border-b border-border bg-muted/40 px-4 py-2 text-[11px] font-semibold text-muted-foreground uppercase tracking-wider select-none shrink-0">
          <div className="flex-1">Work Item Details</div>
          <div className="hidden sm:flex items-center gap-4 justify-end text-right">
            <span className="w-24 text-center">SLA</span>
            <span className="w-24 text-center">Status</span>
            <span className="w-20 text-center">Assignee</span>
            <span className="w-16 text-center">Action</span>
          </div>
        </div>
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
                    className="flex flex-col sm:flex-row sm:items-center justify-between p-3.5 px-4 gap-2.5 transition-colors hover:bg-muted/40 border-b border-border/80 bg-card"
                  >
                    {/* Item Information */}
                    <div className="flex items-start gap-3 min-w-0 flex-1">
                      {/* Priority Badge */}
                      <div className="shrink-0 pt-0.5">{getPriorityBadge(item.priority)}</div>

                      <div className="min-w-0 flex-1 space-y-1">
                        <div className="flex items-center gap-2">
                          <Link
                            href={`/items/${item.id}`}
                            className="font-semibold text-xs sm:text-sm text-foreground truncate hover:text-primary hover:underline underline-offset-2 transition-colors"
                          >
                            {item.title}
                          </Link>
                        </div>

                        <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                          {(() => {
                            const name = item.team_name || memberships.find((m) => m.team_id === item.team_id)?.team_name
                            return (
                              <span className="flex items-center gap-1 font-medium text-foreground/80">
                                <Building2 className="size-3 text-muted-foreground" />
                                {name || "Operations"}
                              </span>
                            )
                          })()}
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
                    <div className="flex items-center justify-between sm:justify-end gap-4 shrink-0 pt-1 sm:pt-0">
                      {/* SLA Badge */}
                      <div className="sm:w-24 flex sm:justify-center">
                        {getSlaBadge(item.sla_state)}
                      </div>

                      {/* Status Badge */}
                      <div className="sm:w-24 flex sm:justify-center">
                        {getStatusBadge(item.status)}
                      </div>

                      {/* Assignee Avatar */}
                      <div className="sm:w-20 flex sm:justify-center">
                        {item.assignee_id ? (
                          <div
                            className="flex items-center gap-1.5"
                            title={`Assigned to ${item.assignee_name || item.assignee_id}`}
                          >
                            <Avatar className="size-6 border border-border">
                              <AvatarFallback className="text-[10px] font-bold bg-primary/10 text-primary">
                                {item.assignee_name
                                  ? item.assignee_name.trim().charAt(0).toUpperCase()
                                  : "O"}
                              </AvatarFallback>
                            </Avatar>
                          </div>
                        ) : (
                          <span className="text-[11px] text-muted-foreground italic">
                            Unassigned
                          </span>
                        )}
                      </div>

                      {/* Inline Claim Button */}
                      <div className="sm:w-16 flex sm:justify-end">
                        {canClaim && (
                          <Button
                            size="xs"
                            variant="outline"
                            onClick={() => claimMutation.mutate(item)}
                            disabled={isClaiming}
                            className="h-7 text-xs font-semibold gap-1 rounded-sm border border-primary/50 text-primary hover:bg-primary hover:text-primary-foreground transition-colors shadow-none"
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
