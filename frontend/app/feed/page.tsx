"use client"

import * as React from "react"
import Link from "next/link"
import { useInfiniteQuery } from "@tanstack/react-query"
import { useAuth } from "@/providers/auth-provider"
import { apiFetch } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { FeedPage, FeedEvent } from "@/types"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Activity,
  PlusCircle,
  UserCheck,
  UserPlus,
  RefreshCw,
  MessageSquare,
  FileEdit,
  ExternalLink,
  Loader2,
  Building2,
} from "lucide-react"

function formatRelative(dateString: string): string {
  try {
    const diffSec = Math.floor((Date.now() - new Date(dateString).getTime()) / 1000)
    if (diffSec < 60) return "just now"
    const mins = Math.floor(diffSec / 60)
    if (mins < 60) return `${mins}m ago`
    const hours = Math.floor(mins / 60)
    if (hours < 24) return `${hours}h ago`
    const days = Math.floor(hours / 24)
    return `${days}d ago`
  } catch {
    return dateString
  }
}

export default function ActivityFeedPage() {
  const { memberships } = useAuth()
  const [selectedTeam, setSelectedTeam] = React.useState<string>("all")

  // Infinite query for feed events
  const {
    data,
    isLoading,
    isFetchingNextPage,
    hasNextPage,
    fetchNextPage,
    refetch,
    isRefetching,
  } = useInfiniteQuery<FeedPage>({
    queryKey: queryKeys.feed.list(selectedTeam === "all" ? undefined : selectedTeam),
    queryFn: async ({ pageParam = "" }) => {
      const teamParam = selectedTeam !== "all" ? `&team=${selectedTeam}` : ""
      const cursorParam = pageParam ? `&cursor=${pageParam}` : ""
      return await apiFetch<FeedPage>(`/feed?limit=25${teamParam}${cursorParam}`)
    },
    getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
    initialPageParam: "",
  })

  const allEvents: FeedEvent[] = React.useMemo(() => {
    return data?.pages.flatMap((page) => page.events) || []
  }, [data])

  const getEventIcon = (type: string) => {
    switch (type) {
      case "created":
        return <PlusCircle className="size-4 text-emerald-500" />
      case "claimed":
        return <UserCheck className="size-4 text-blue-500" />
      case "assigned":
        return <UserPlus className="size-4 text-purple-500" />
      case "status_changed":
        return <RefreshCw className="size-4 text-amber-500" />
      case "commented":
        return <MessageSquare className="size-4 text-cyan-500" />
      default:
        return <FileEdit className="size-4 text-muted-foreground" />
    }
  }

  const formatEventAction = (ev: FeedEvent) => {
    switch (ev.type) {
      case "created":
        return "created work item"
      case "claimed":
        return "claimed work item"
      case "assigned":
        return "reassigned work item"
      case "status_changed":
        return "updated status"
      case "commented":
        return "commented on work item"
      default:
        return `performed ${ev.type}`
    }
  }

  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight text-foreground sm:text-3xl">
              Activity Feed
            </h1>
            <Badge variant="outline" className="text-xs">
              Live Audits
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground mt-0.5">
            Real-time chronological timeline of operational transitions, assignments, and claims.
          </p>
        </div>

        <div className="flex items-center gap-2">
          {memberships.length > 0 && (
            <Select
              value={selectedTeam}
              onValueChange={(val) => {
                if (val) setSelectedTeam(val)
              }}
            >
              <SelectTrigger className="w-48 h-9 text-xs">
                <Building2 className="size-3.5 mr-1 text-muted-foreground" />
                <SelectValue placeholder="All teams" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Accessible Teams</SelectItem>
                {memberships.map((m) => (
                  <SelectItem key={m.team_id} value={m.team_id}>
                    {m.team_name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}

          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            disabled={isRefetching}
            className="h-9 gap-1.5 text-xs"
          >
            <RefreshCw className={`size-3.5 ${isRefetching ? "animate-spin" : ""}`} />
            Refresh
          </Button>
        </div>
      </div>

      <Card>
        <CardHeader className="pb-3 border-b">
          <div className="flex items-center justify-between">
            <CardTitle className="text-base font-semibold flex items-center gap-2">
              <Activity className="size-4 text-primary" />
              Recent Operations
            </CardTitle>
            <Badge variant="secondary" className="text-xs">
              {allEvents.length} events loaded
            </Badge>
          </div>
          <CardDescription className="text-xs">
            Audit logs and team interactions across your operations workspace.
          </CardDescription>
        </CardHeader>

        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex items-center justify-center p-12 text-sm text-muted-foreground gap-2">
              <Loader2 className="size-5 animate-spin text-primary" />
              Loading activity feed...
            </div>
          ) : allEvents.length === 0 ? (
            <div className="flex flex-col items-center justify-center p-12 text-center text-muted-foreground">
              <Activity className="size-10 text-muted-foreground/30 mb-3" />
              <p className="text-sm font-semibold text-foreground">No recent activity</p>
              <p className="text-xs text-muted-foreground mt-1 max-w-sm">
                Operational changes and comments will be streamed here in real time.
              </p>
            </div>
          ) : (
            <div className="divide-y">
              {allEvents.map((ev) => (
                <div
                  key={ev.id}
                  className="flex items-start gap-3.5 p-4 transition-colors hover:bg-muted/40"
                >
                  {/* Actor Avatar */}
                  <Avatar className="size-8 border shrink-0 mt-0.5">
                    <AvatarFallback className="text-[11px] font-bold bg-primary/10 text-primary">
                      {ev.actor_name ? ev.actor_name.slice(0, 2).toUpperCase() : "OP"}
                    </AvatarFallback>
                  </Avatar>

                  {/* Event Details */}
                  <div className="flex-1 min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-1.5 text-xs text-foreground">
                      <span className="font-bold text-foreground">
                        {ev.actor_name || "Operator"}
                      </span>
                      <span className="text-muted-foreground">
                        {formatEventAction(ev)}
                      </span>
                      <Link
                        href={`/items/${ev.item_id}`}
                        className="font-semibold text-primary hover:underline inline-flex items-center gap-1 truncate max-w-xs"
                      >
                        {ev.item_title || `#${ev.item_id.slice(0, 8)}`}
                        <ExternalLink className="size-2.5" />
                      </Link>
                    </div>

                    {/* Subtitle / Reason / Field Details */}
                    {ev.reason && (
                      <p className="text-xs text-muted-foreground bg-muted/50 rounded-md p-1.5 font-mono text-[11px] border">
                        Reason: {ev.reason}
                      </p>
                    )}

                    {ev.field && (
                      <p className="text-[11px] text-muted-foreground">
                        Updated field: <span className="font-semibold">{ev.field}</span>
                      </p>
                    )}

                    <div className="flex items-center gap-2 text-[10px] text-muted-foreground pt-0.5">
                      <span>{formatRelative(ev.created_at)}</span>
                      <span>•</span>
                      <span className="font-mono text-[9px] uppercase">
                        EVENT #{ev.id}
                      </span>
                    </div>
                  </div>

                  {/* Event Type Icon badge */}
                  <div className="shrink-0 p-1.5 rounded-full bg-muted/60">
                    {getEventIcon(ev.type)}
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* Infinite Scroll Load More Button */}
          {hasNextPage && (
            <div className="p-4 border-t text-center bg-muted/20">
              <Button
                variant="outline"
                size="sm"
                onClick={() => fetchNextPage()}
                disabled={isFetchingNextPage}
                className="w-full sm:w-auto text-xs font-semibold gap-1.5"
              >
                {isFetchingNextPage ? (
                  <>
                    <Loader2 className="size-3.5 animate-spin" />
                    Loading older events...
                  </>
                ) : (
                  "Load More Events"
                )}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
