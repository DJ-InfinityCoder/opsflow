"use client"

import * as React from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useAuth } from "@/providers/auth-provider"
import { apiFetch } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { Notification, NotificationPage } from "@/types"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Bell,
  CheckCheck,
  Check,
  UserCheck,
  AtSign,
  AlertTriangle,
  Loader2,
  ExternalLink,
  Inbox,
  RefreshCw,
} from "lucide-react"
import { toast } from "sonner"

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

export default function NotificationsPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { user } = useAuth()
  const [filter, setFilter] = React.useState<"all" | "unread">("all")

  // Query notifications
  const { data, isLoading, refetch } = useQuery<NotificationPage>({
    queryKey: queryKeys.notifications.list(filter === "unread"),
    queryFn: () =>
      apiFetch<NotificationPage>(
        `/notifications?unread_only=${filter === "unread"}&limit=50`
      ),
    refetchInterval: 30_000,
    refetchOnWindowFocus: true,
  })

  const notifications = data?.notifications || []
  const unreadCount = notifications.filter((n) => !n.read_at).length

  // Mark as read mutation
  const markReadMutation = useMutation({
    mutationFn: async (ids: string[]) => {
      return await apiFetch<{ updated_count: number }>("/notifications/read", {
        method: "POST",
        body: JSON.stringify({ ids }),
      })
    },
    onSuccess: () => {
      toast.success("Notifications marked as read")
      queryClient.invalidateQueries({ queryKey: queryKeys.notifications.all })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to mark as read")
    },
  })

  const handleMarkAllRead = () => {
    const unreadIds = notifications.filter((n) => !n.read_at).map((n) => n.id)
    if (unreadIds.length === 0) return
    markReadMutation.mutate(unreadIds)
  }

  const handleNotificationClick = (notif: Notification) => {
    if (!notif.read_at) {
      markReadMutation.mutate([notif.id])
    }
    if (notif.item_id) {
      router.push(`/items/${notif.item_id}`)
    }
  }

  const getNotificationIcon = (type: string) => {
    switch (type) {
      case "assignment":
        return <UserCheck className="size-4 text-blue-500" />
      case "mention":
        return <AtSign className="size-4 text-purple-500" />
      case "p1_created":
        return <AlertTriangle className="size-4 text-red-500" />
      default:
        return <Bell className="size-4 text-amber-500" />
    }
  }

  const getNotificationTitle = (notif: Notification) => {
    if (notif.title) return notif.title
    switch (notif.type) {
      case "assignment":
        return "Work item assigned to you"
      case "mention":
        return "You were @mentioned in a discussion"
      case "p1_created":
        return "Critical P1 incident logged"
      default:
        return `Notification: ${notif.type}`
    }
  }

  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-foreground sm:text-3xl">
            Notifications
          </h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            Activity updates and alerts for {user?.email}.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            className="h-8 gap-1.5 text-xs font-semibold"
          >
            <RefreshCw className="size-3 text-muted-foreground" />
            Refresh
          </Button>

          {unreadCount > 0 && (
            <Button
              variant="outline"
              size="sm"
              onClick={handleMarkAllRead}
              disabled={markReadMutation.isPending}
              className="h-8 gap-1.5 text-xs font-semibold"
            >
              {markReadMutation.isPending ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <CheckCheck className="size-3.5 text-primary" />
              )}
              Mark all as read
            </Button>
          )}
        </div>
      </div>

      <Card>
        <CardHeader className="pb-3 border-b">
          <div className="flex items-center justify-between flex-wrap gap-2">
            <div className="flex items-center gap-2">
              <CardTitle className="text-base font-semibold">Alert Inbox</CardTitle>
              {unreadCount > 0 && (
                <Badge variant="destructive" className="text-xs">
                  {unreadCount} unread
                </Badge>
              )}
            </div>

            <div className="flex items-center gap-2">
              <Tabs
                value={filter}
                onValueChange={(val) => setFilter(val as "all" | "unread")}
              >
                <TabsList className="h-8">
                  <TabsTrigger value="all" className="text-xs px-3">
                    All
                  </TabsTrigger>
                  <TabsTrigger value="unread" className="text-xs px-3">
                    Unread Only
                  </TabsTrigger>
                </TabsList>
              </Tabs>
            </div>
          </div>
        </CardHeader>

        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex items-center justify-center p-12 text-sm text-muted-foreground gap-2">
              <Loader2 className="size-4 animate-spin text-primary" />
              Loading notifications...
            </div>
          ) : notifications.length === 0 ? (
            <div className="flex flex-col items-center justify-center p-12 text-center text-muted-foreground">
              <Inbox className="size-10 text-muted-foreground/30 mb-3" />
              <p className="text-sm font-semibold text-foreground">
                {filter === "unread" ? "No unread notifications" : "No notifications yet"}
              </p>
              <p className="text-xs text-muted-foreground mt-1 max-w-sm">
                When you are assigned to work items, mentioned in comments, or P1 incidents occur, you will be alerted here.
              </p>
            </div>
          ) : (
            <div className="divide-y">
              {notifications.map((notif) => {
                const isUnread = !notif.read_at
                return (
                  <div
                    key={notif.id}
                    className={`flex items-start justify-between gap-3 p-4 transition-colors hover:bg-muted/40 cursor-pointer ${
                      isUnread ? "bg-primary/5" : ""
                    }`}
                    onClick={() => handleNotificationClick(notif)}
                  >
                    <div className="flex items-start gap-3 min-w-0 flex-1">
                      <div className="mt-0.5 shrink-0 rounded-full bg-muted/80 p-2">
                        {getNotificationIcon(notif.type)}
                      </div>

                      <div className="space-y-1 min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <span
                            className={`text-sm ${
                              isUnread ? "font-bold text-foreground" : "font-medium text-foreground"
                            }`}
                          >
                            {getNotificationTitle(notif)}
                          </span>
                          {isUnread && (
                            <span className="size-2 rounded-full bg-primary shrink-0" />
                          )}
                        </div>

                        {notif.body && (
                          <p className="text-xs text-muted-foreground">
                            {notif.body}
                          </p>
                        )}

                        <div className="flex items-center gap-3 text-xs text-muted-foreground pt-1">
                          <span>{formatRelative(notif.created_at)}</span>
                          {notif.item_id && (
                            <>
                              <span>•</span>
                              <Link
                                href={`/items/${notif.item_id}`}
                                onClick={(e) => e.stopPropagation()}
                                className="font-mono text-[11px] text-primary hover:underline flex items-center gap-1"
                              >
                                #{notif.item_id.slice(0, 8)}
                                <ExternalLink className="size-3" />
                              </Link>
                            </>
                          )}
                        </div>
                      </div>
                    </div>

                    <div className="shrink-0 flex items-center gap-2">
                      {isUnread && (
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={(e) => {
                            e.stopPropagation()
                            markReadMutation.mutate([notif.id])
                          }}
                          title="Mark as read"
                          className="text-muted-foreground hover:text-foreground"
                        >
                          <Check className="size-3.5" />
                        </Button>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
