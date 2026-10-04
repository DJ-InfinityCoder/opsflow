"use client"

import * as React from "react"
import { useRouter } from "next/navigation"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { apiFetch } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { Notification, NotificationPage } from "@/types"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Bell,
  CheckCheck,
  UserCheck,
  AtSign,
  AlertTriangle,
  Loader2,
  ExternalLink,
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

export function NotificationsPopover() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const [open, setOpen] = React.useState(false)

  // Query notifications with 30s poll and on focus
  const { data, isLoading } = useQuery<NotificationPage>({
    queryKey: queryKeys.notifications.list(false),
    queryFn: () => apiFetch<NotificationPage>("/notifications?limit=20"),
    refetchInterval: 30_000,
    refetchOnWindowFocus: true,
  })

  const notifications = data?.notifications || []
  const unreadNotifications = notifications.filter((n) => !n.read_at)
  const unreadCount = data?.unread_count ?? unreadNotifications.length

  // Mutation to mark notifications as read
  const markReadMutation = useMutation({
    mutationFn: async (ids: string[]) => {
      return await apiFetch<{ updated_count: number }>("/notifications/read", {
        method: "POST",
        body: JSON.stringify({ ids }),
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.notifications.all })
    },
  })

  const handleNotificationClick = (notif: Notification) => {
    if (!notif.read_at) {
      markReadMutation.mutate([notif.id])
    }
    setOpen(false)
    if (notif.item_id) {
      router.push(`/items/${notif.item_id}`)
    }
  }

  const handleMarkAllRead = () => {
    if (unreadCount === 0) return
    markReadMutation.mutate([])
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
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            variant="outline"
            size="icon-sm"
            className="relative size-8 rounded-sm border border-border bg-background hover:bg-accent text-foreground transition-colors"
            aria-label="Notifications"
          />
        }
      >
        <Bell className="size-4 text-foreground/80" />
        {unreadCount > 0 && (
          <span className="absolute -top-1 -right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[9px] font-bold text-destructive-foreground border-2 border-background animate-in zoom-in-50">
            {unreadCount > 9 ? "9+" : unreadCount}
          </span>
        )}
      </PopoverTrigger>

      <PopoverContent align="end" className="w-80 sm:w-96 p-0 border border-border shadow-none rounded-md">
        {/* Header */}
        <div className="flex items-center justify-between border-b px-4 py-3">
          <div className="flex items-center gap-2">
            <span className="font-bold text-sm">Notifications</span>
            {unreadCount > 0 ? (
              <Badge variant="destructive" className="h-5 px-1.5 text-[10px]">
                {unreadCount} unread
              </Badge>
            ) : (
              <Badge variant="secondary" className="h-5 px-1.5 text-[10px]">
                All read
              </Badge>
            )}
          </div>

          {unreadCount > 0 && (
            <Button
              variant="ghost"
              size="xs"
              onClick={handleMarkAllRead}
              disabled={markReadMutation.isPending}
              className="text-[11px] text-muted-foreground hover:text-foreground gap-1"
            >
              {markReadMutation.isPending ? (
                <Loader2 className="size-3 animate-spin" />
              ) : (
                <CheckCheck className="size-3 text-primary" />
              )}
              Mark all read
            </Button>
          )}
        </div>

        {/* List */}
        <div className="max-h-[360px] overflow-y-auto divide-y">
          {isLoading ? (
            <div className="flex items-center justify-center p-8 text-xs text-muted-foreground gap-2">
              <Loader2 className="size-4 animate-spin text-primary" />
              Loading notifications...
            </div>
          ) : notifications.length === 0 ? (
            <div className="flex flex-col items-center justify-center p-8 text-center text-muted-foreground">
              <Bell className="size-8 text-muted-foreground/30 mb-2" />
              <p className="text-xs font-semibold">No notifications yet</p>
              <p className="text-[11px] text-muted-foreground mt-0.5">
                Assignments, mentions, and P1 alerts will show up here.
              </p>
            </div>
          ) : (
            notifications.map((notif) => {
              const isUnread = !notif.read_at
              return (
                <div
                  key={notif.id}
                  onClick={() => handleNotificationClick(notif)}
                  className={`flex items-start gap-3 p-3 transition-colors cursor-pointer hover:bg-muted/50 ${
                    isUnread ? "bg-primary/5" : ""
                  }`}
                >
                  <div className="mt-0.5 shrink-0 rounded-full bg-muted/80 p-1.5">
                    {getNotificationIcon(notif.type)}
                  </div>

                  <div className="flex-1 min-w-0 space-y-0.5">
                    <div className="flex items-center justify-between gap-1">
                      <p
                        className={`text-xs truncate ${
                          isUnread ? "font-bold text-foreground" : "font-medium text-foreground"
                        }`}
                      >
                        {getNotificationTitle(notif)}
                      </p>
                      {isUnread && (
                        <span className="size-2 shrink-0 rounded-full bg-primary" />
                      )}
                    </div>

                    {notif.body && (
                      <p className="text-[11px] text-muted-foreground line-clamp-2">
                        {notif.body}
                      </p>
                    )}

                    <div className="flex items-center justify-between text-[10px] text-muted-foreground pt-1">
                      <span>{formatRelative(notif.created_at)}</span>
                      {notif.item_id && (
                        <span className="font-mono text-[9px] hover:underline flex items-center gap-0.5 text-primary">
                          #{notif.item_id.slice(0, 8)}
                          <ExternalLink className="size-2.5" />
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              )
            })
          )}
        </div>

        {/* Footer */}
        <div className="border-t p-2 text-center bg-muted/20">
          <Button
            variant="ghost"
            size="xs"
            onClick={() => {
              setOpen(false)
              router.push("/notifications")
            }}
            className="w-full text-xs text-muted-foreground hover:text-foreground"
          >
            View all notifications
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
