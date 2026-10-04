"use client"

import * as React from "react"
import { useParams } from "next/navigation"
import Link from "next/link"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { useAuth } from "@/providers/auth-provider"
import { apiFetch, ApiError } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import { useIdempotencyKey } from "@/hooks/use-idempotency-key"
import type {
  WorkItem,
  ItemEvent,
  Comment,
  TeamFieldSchema,
  TeamMember,
  ItemStatus,
} from "@/types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { toast } from "sonner"
import {
  ArrowLeft,
  AlertTriangle,
  Clock,
  CheckCircle2,
  XCircle,
  RefreshCw,
  Send,
  MessageSquare,
  History,
  ShieldAlert,
  Loader2,
  Check,
  X,
  FileEdit,
  Sparkles,
  ArrowRight,
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
        <Badge variant="outline" className="border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400 font-bold text-xs px-2 py-0.5">
          P1 Critical
        </Badge>
      )
    case 2:
      return (
        <Badge variant="outline" className="border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400 font-semibold text-xs px-2 py-0.5">
          P2 High
        </Badge>
      )
    case 3:
      return (
        <Badge variant="outline" className="border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-400 font-medium text-xs px-2 py-0.5">
          P3 Medium
        </Badge>
      )
    default:
      return (
        <Badge variant="outline" className="border-zinc-500/30 bg-zinc-500/10 text-zinc-600 dark:text-zinc-400 text-xs px-2 py-0.5">
          P4 Low
        </Badge>
      )
  }
}

function getStatusBadge(status: string) {
  switch (status) {
    case "new":
      return <Badge variant="outline" className="border-sky-500/30 bg-sky-500/10 text-sky-600 dark:text-sky-400 text-xs capitalize px-2.5 py-0.5">New</Badge>
    case "triaged":
      return <Badge variant="outline" className="border-purple-500/30 bg-purple-500/10 text-purple-600 dark:text-purple-400 text-xs capitalize px-2.5 py-0.5">Triaged</Badge>
    case "in_progress":
      return <Badge variant="outline" className="border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400 text-xs capitalize px-2.5 py-0.5">In Progress</Badge>
    case "pending_approval":
      return <Badge variant="outline" className="border-orange-500/30 bg-orange-500/10 text-orange-600 dark:text-orange-400 text-xs capitalize px-2.5 py-0.5">Pending Approval</Badge>
    case "resolved":
      return <Badge variant="outline" className="border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 text-xs capitalize px-2.5 py-0.5">Resolved</Badge>
    case "closed":
      return <Badge variant="outline" className="border-zinc-500/30 bg-zinc-500/10 text-zinc-600 dark:text-zinc-400 text-xs capitalize px-2.5 py-0.5">Closed</Badge>
    default:
      return <Badge variant="outline" className="text-xs capitalize px-2.5 py-0.5">{status}</Badge>
  }
}

function getSlaBadge(slaState?: string) {
  if (!slaState) return null
  switch (slaState) {
    case "ok":
      return (
        <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-0.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
          <CheckCircle2 className="size-3.5" />
          SLA OK
        </span>
      )
    case "warning":
      return (
        <span className="inline-flex items-center gap-1.5 rounded-full bg-amber-500/10 border border-amber-500/20 px-2.5 py-0.5 text-xs font-semibold text-amber-600 dark:text-amber-400">
          <Clock className="size-3.5" />
          SLA Warning
        </span>
      )
    case "breached":
      return (
        <span className="inline-flex items-center gap-1.5 rounded-full bg-red-500/10 border border-red-500/20 px-2.5 py-0.5 text-xs font-bold text-red-600 dark:text-red-400">
          <AlertTriangle className="size-3.5" />
          SLA Breached
        </span>
      )
    default:
      return null
  }
}

function formatStatusLabel(status: string): string {
  switch (status) {
    case "triaged":
      return "Triage Item"
    case "in_progress":
      return "Start Progress"
    case "pending_approval":
      return "Request Approval"
    case "resolved":
      return "Resolve Item"
    case "closed":
      return "Close Item"
    default:
      return status.replace(/_/g, " ")
  }
}

interface TimelineItem {
  id: string
  type: "event" | "comment"
  created_at: string
  title: string
  subtitle?: string
  authorName?: string
  body?: string
  actorId?: string
}

function ItemDetailContent({
  initialItem,
  itemID,
}: {
  initialItem: WorkItem
  itemID: string
}) {
  const queryClient = useQueryClient()
  const { user, isLead } = useAuth()

  // 1. Fetch live Work Item with 10s polling and focus refetching
  const { data: item = initialItem, refetch: refetchItem } = useQuery<WorkItem>({
    queryKey: queryKeys.items.detail(itemID),
    queryFn: () => apiFetch<WorkItem>(`/items/${itemID}`),
    initialData: initialItem,
    refetchInterval: 10000,
    refetchOnWindowFocus: true,
  })

  // 2. Fetch Field Schemas for dynamic custom fields
  const { data: fieldSchemas } = useQuery<TeamFieldSchema[]>({
    queryKey: queryKeys.teams.schemas(item.team_id),
    queryFn: () => apiFetch<TeamFieldSchema[]>(`/teams/${item.team_id}/field-schemas`),
    enabled: !!item.team_id,
  })

  // 3. Fetch Team Members for @mentions
  const { data: teamMembers } = useQuery<TeamMember[]>({
    queryKey: queryKeys.teams.members(item.team_id),
    queryFn: () => apiFetch<TeamMember[]>(`/teams/${item.team_id}/members`),
    enabled: !!item.team_id,
  })

  // 4. Fetch Events & Comments
  const { data: eventsData, refetch: refetchEvents } = useQuery<{ events: ItemEvent[] }>({
    queryKey: queryKeys.items.events(itemID),
    queryFn: () => apiFetch<{ events: ItemEvent[] }>(`/items/${itemID}/events`),
  })

  const { data: commentsData, refetch: refetchComments } = useQuery<{ comments: Comment[] }>({
    queryKey: queryKeys.items.comments(itemID),
    queryFn: () => apiFetch<{ comments: Comment[] }>(`/items/${itemID}/comments`),
  })

  // Local draft state for Edit Form initialized directly without useEffect
  const [draftTitle, setDraftTitle] = React.useState(initialItem.title)
  const [draftDescription, setDraftDescription] = React.useState(initialItem.description || "")
  const [draftPriority, setDraftPriority] = React.useState<number>(initialItem.priority)
  const [draftCustomFields, setDraftCustomFields] = React.useState<Record<string, unknown>>(
    initialItem.custom_fields || {}
  )
  const [loadedVersion, setLoadedVersion] = React.useState<number>(initialItem.version)
  const [isFormDirty, setIsFormDirty] = React.useState(false)

  // Idempotency keys per user intent; new edits/actions get a new key.
  const editIdempotencyKey = useIdempotencyKey()
  const actionIdempotencyKey = useIdempotencyKey()

  // Reset idempotency keys when inputs change
  const handleInputChange = () => {
    setIsFormDirty(true)
    editIdempotencyKey.resetForNewIntent()
  }

  // Reason Dialog state for transitions requiring justification
  const [reasonDialogOpen, setReasonDialogOpen] = React.useState(false)
  const [pendingTargetState, setPendingTargetState] = React.useState<string | null>(null)
  const [transitionReason, setTransitionReason] = React.useState("")

  // Approval Decision Dialog state (Approve / Reject)
  const [approvalDialogOpen, setApprovalDialogOpen] = React.useState(false)
  const [approvalDecision, setApprovalDecision] = React.useState<"approved" | "rejected">("approved")
  const [approvalReason, setApprovalReason] = React.useState("")

  // Version Conflict Resolution Dialog state
  const [conflictDialogOpen, setConflictDialogOpen] = React.useState(false)
  const [conflictServerItem, setConflictServerItem] = React.useState<WorkItem | null>(null)
  const [conflictFieldSelections, setConflictFieldSelections] = React.useState<Record<string, "mine" | "theirs">>({})

  // Staleness detection: server version > loaded version
  const isStale = item && loadedVersion > 0 && item.version > loadedVersion

  const reloadServerVersion = () => {
    if (item) {
      setDraftTitle(item.title)
      setDraftDescription(item.description || "")
      setDraftPriority(item.priority)
      setDraftCustomFields(item.custom_fields || {})
      setLoadedVersion(item.version)
      setIsFormDirty(false)
      editIdempotencyKey.resetForNewIntent()
      toast.info(`Updated draft to latest server version (v${item.version})`)
    }
  }

  // --- MUTATION: Save / Edit Item (PATCH) with If-Match ---
  const saveMutation = useMutation({
    mutationFn: async ({
      patchPayload,
      versionToMatch,
    }: {
      patchPayload: Record<string, unknown>
      versionToMatch: number
    }) => {
      return apiFetch<WorkItem>(`/items/${itemID}`, {
        method: "PATCH",
        body: patchPayload,
        ifMatch: versionToMatch,
        idempotencyKey: editIdempotencyKey.read(),
      })
    },
    onSuccess: (updated) => {
      toast.success("Item saved successfully")
      setLoadedVersion(updated.version)
      setDraftTitle(updated.title)
      setDraftDescription(updated.description || "")
      setDraftPriority(updated.priority)
      setDraftCustomFields(updated.custom_fields || {})
      setIsFormDirty(false)
      editIdempotencyKey.resetForNewIntent()

      queryClient.setQueryData(queryKeys.items.detail(itemID), updated)
      queryClient.invalidateQueries({ queryKey: queryKeys.items.events(itemID) })
      queryClient.invalidateQueries({ queryKey: queryKeys.items.lists() })
      queryClient.invalidateQueries({ queryKey: queryKeys.views.counts() })
    },
    onError: (err: unknown) => {
      if (err instanceof ApiError && err.status === 409) {
        // Version conflict!
        const serverItem = err.details?.current_item as WorkItem | undefined
        if (serverItem) {
          setConflictServerItem(serverItem)
          setConflictFieldSelections({
            title: "mine",
            description: "mine",
            priority: "mine",
          })
          setConflictDialogOpen(true)
          return
        }
      }
      toast.error(err instanceof Error ? err.message : "Failed to save changes")
    },
  })

  const handleSaveForm = (e: React.FormEvent) => {
    e.preventDefault()
    if (!item) return

    const payload: Record<string, unknown> = {
      title: draftTitle,
      description: draftDescription,
      priority: draftPriority,
      custom_fields: draftCustomFields,
    }

    saveMutation.mutate({
      patchPayload: payload,
      versionToMatch: loadedVersion,
    })
  }

  const handleResolveConflictAndSave = () => {
    if (!conflictServerItem) return

    const resolvedTitle =
      conflictFieldSelections.title === "mine" ? draftTitle : conflictServerItem.title
    const resolvedDescription =
      conflictFieldSelections.description === "mine"
        ? draftDescription
        : conflictServerItem.description || ""
    const resolvedPriority =
      conflictFieldSelections.priority === "mine"
        ? draftPriority
        : conflictServerItem.priority

    const payload: Record<string, unknown> = {
      title: resolvedTitle,
      description: resolvedDescription,
      priority: resolvedPriority,
      custom_fields: draftCustomFields,
    }

    setConflictDialogOpen(false)
    saveMutation.mutate({
      patchPayload: payload,
      versionToMatch: conflictServerItem.version,
    })
  }

  // --- MUTATION: State Machine Transition (POST /items/:id/transition) ---
  const transitionMutation = useMutation({
    mutationFn: async ({
      targetState,
      reason,
    }: {
      targetState: string
      reason?: string
    }) => {
      return apiFetch<WorkItem>(`/items/${itemID}/transition`, {
        method: "POST",
        body: { to: targetState, reason },
        ifMatch: item.version,
        idempotencyKey: actionIdempotencyKey.read(),
      })
    },
    onMutate: async ({ targetState }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.items.detail(itemID) })
      const previous = queryClient.getQueryData<WorkItem>(queryKeys.items.detail(itemID))

      if (previous) {
        queryClient.setQueryData<WorkItem>(queryKeys.items.detail(itemID), {
          ...previous,
          status: targetState as ItemStatus,
          version: previous.version + 1,
        })
      }

      return { previous }
    },
    onError: (err, _, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKeys.items.detail(itemID), context.previous)
      }
      toast.error(err instanceof Error ? err.message : "Failed to change item status")
    },
    onSuccess: (updated) => {
      toast.success(`Item moved to ${formatStatusLabel(updated.status)}`)
      actionIdempotencyKey.resetForNewIntent()
      setLoadedVersion(updated.version)
      queryClient.setQueryData(queryKeys.items.detail(itemID), updated)
      queryClient.invalidateQueries({ queryKey: queryKeys.items.events(itemID) })
      queryClient.invalidateQueries({ queryKey: queryKeys.items.lists() })
      queryClient.invalidateQueries({ queryKey: queryKeys.views.counts() })
    },
  })

  const triggerTransition = (targetState: string) => {
    actionIdempotencyKey.resetForNewIntent()
    const requiresReason =
      targetState === "pending_approval" ||
      (item.status === "resolved" && targetState === "in_progress") ||
      (item.status === "pending_approval" && targetState === "in_progress")

    if (requiresReason) {
      setPendingTargetState(targetState)
      setTransitionReason("")
      setReasonDialogOpen(true)
    } else {
      transitionMutation.mutate({ targetState })
    }
  }

  const handleReasonDialogSubmit = () => {
    if (!pendingTargetState) return
    if (!transitionReason.trim()) {
      toast.error("A reason is required for this transition.")
      return
    }

    setReasonDialogOpen(false)
    transitionMutation.mutate({
      targetState: pendingTargetState,
      reason: transitionReason.trim(),
    })
  }

  // --- MUTATION: Decide Approval (Approve / Reject) ---
  const decideApprovalMutation = useMutation({
    mutationFn: async ({
      approvalID,
      decision,
      reason,
    }: {
      approvalID: string
      decision: "approved" | "rejected"
      reason: string
    }) => {
      return apiFetch<WorkItem>(`/items/${itemID}/approvals/${approvalID}/decide`, {
        method: "POST",
        body: { decision, reason },
        idempotencyKey: actionIdempotencyKey.read(),
      })
    },
    onMutate: async ({ decision }) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.items.detail(itemID) })
      const previous = queryClient.getQueryData<WorkItem>(queryKeys.items.detail(itemID))

      if (previous) {
        queryClient.setQueryData<WorkItem>(queryKeys.items.detail(itemID), {
          ...previous,
          status: decision === "approved" ? "resolved" : "in_progress",
          version: previous.version + 1,
        })
      }
      return { previous }
    },
    onError: (err, _, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKeys.items.detail(itemID), context.previous)
      }
      toast.error(err instanceof Error ? err.message : "Approval decision failed")
    },
    onSuccess: (updated, { decision }) => {
      toast.success(`Approval ${decision === "approved" ? "approved" : "rejected"} successfully`)
      actionIdempotencyKey.resetForNewIntent()
      setLoadedVersion(updated.version)
      queryClient.setQueryData(queryKeys.items.detail(itemID), updated)
      queryClient.invalidateQueries({ queryKey: queryKeys.items.events(itemID) })
      queryClient.invalidateQueries({ queryKey: queryKeys.items.lists() })
      queryClient.invalidateQueries({ queryKey: queryKeys.views.counts() })
    },
  })

  const triggerApprovalDecision = (decision: "approved" | "rejected") => {
    actionIdempotencyKey.resetForNewIntent()
    setApprovalDecision(decision)
    setApprovalReason("")
    setApprovalDialogOpen(true)
  }

  const handleApprovalDialogSubmit = () => {
    const approvalID = item.pending_approval?.id
    if (!approvalID) {
      toast.error("Pending approval ID not found.")
      return
    }
    if (!approvalReason.trim()) {
      toast.error("Reason is required.")
      return
    }

    setApprovalDialogOpen(false)
    decideApprovalMutation.mutate({
      approvalID,
      decision: approvalDecision,
      reason: approvalReason.trim(),
    })
  }

  // --- COMMENTS & @MENTION AUTOCOMPLETE ---
  const [commentText, setCommentText] = React.useState("")
  const [mentionQuery, setMentionQuery] = React.useState<string | null>(null)
  const [mentionPosition, setMentionPosition] = React.useState<number>(0)
  const commentInputRef = React.useRef<HTMLTextAreaElement>(null)
  const commentIdempotencyKey = useIdempotencyKey()

  const handleCommentChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const val = e.target.value
    setCommentText(val)
    commentIdempotencyKey.resetForNewIntent()

    const cursor = e.target.selectionStart || 0
    const textBeforeCursor = val.slice(0, cursor)
    const match = textBeforeCursor.match(/@([a-zA-Z0-9._-]*)$/)

    if (match) {
      setMentionQuery(match[1])
      setMentionPosition(cursor)
    } else {
      setMentionQuery(null)
    }
  }

  const insertMention = (member: TeamMember) => {
    const mentionHandle = member.email ? member.email.split("@")[0] : member.name.replace(/\s+/g, "_")
    const before = commentText.slice(0, mentionPosition - (mentionQuery?.length || 0) - 1)
    const after = commentText.slice(mentionPosition)
    const nextText = `${before}@${mentionHandle} ${after}`
    setCommentText(nextText)
    setMentionQuery(null)
    commentInputRef.current?.focus()
  }

  const commentMutation = useMutation({
    mutationFn: async (body: string) => {
      return apiFetch<Comment>(`/items/${itemID}/comments`, {
        method: "POST",
        body: { body },
        idempotencyKey: commentIdempotencyKey.read(),
      })
    },
    onSuccess: (newComment) => {
      toast.success("Comment added")
      setCommentText("")
      commentIdempotencyKey.resetForNewIntent()
      queryClient.setQueryData<{ comments: Comment[] }>(queryKeys.items.comments(itemID), (old) => {
        return {
          comments: [...(old?.comments || []), newComment],
        }
      })
      refetchEvents()
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to post comment")
    },
  })

  const handlePostComment = (e: React.FormEvent) => {
    e.preventDefault()
    if (!commentText.trim()) return
    commentMutation.mutate(commentText.trim())
  }

  // --- MERGED TIMELINE ---
  const timelineItems: TimelineItem[] = React.useMemo(() => {
    const events: TimelineItem[] = (eventsData?.events || []).map((ev) => {
      let title = `Event: ${ev.type}`
      let subtitle = ""

      if (ev.type === "created") {
        title = "Created work item"
      } else if (ev.type === "claimed") {
        title = "Claimed this work item"
      } else if (ev.type === "assigned") {
        title = "Reassigned work item"
        subtitle = ev.reason ? `Reason: ${ev.reason}` : ""
      } else if (ev.type === "status_changed") {
        title = `Status changed to ${ev.new_value ? String(ev.new_value).replace(/"/g, "") : ""}`
        subtitle = ev.reason ? `Reason: ${ev.reason}` : ""
      } else if (ev.field) {
        title = `Updated ${ev.field}`
        subtitle = ev.reason ? `Reason: ${ev.reason}` : ""
      }

      return {
        id: `ev-${ev.id}`,
        type: "event",
        created_at: String(ev.created_at),
        title,
        subtitle,
        actorId: ev.actor_id,
        authorName: ev.actor_name,
      }
    })

    const comments: TimelineItem[] = (commentsData?.comments || []).map((c) => ({
      id: `cm-${c.id}`,
      type: "comment",
      created_at: String(c.created_at),
      title: "Commented",
      authorName: c.author_name || c.author_id.slice(0, 8),
      body: c.body,
      actorId: c.author_id,
    }))

    return [...events, ...comments].sort(
      (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
    )
  }, [eventsData, commentsData])

  const filteredMentions = React.useMemo(() => {
    if (mentionQuery === null || !teamMembers) return []
    const q = mentionQuery.toLowerCase()
    return teamMembers.filter(
      (m) =>
        m.name.toLowerCase().includes(q) ||
        m.email.toLowerCase().includes(q)
    )
  }, [mentionQuery, teamMembers])

  const isEligibleLead =
    isLead &&
    item.status === "pending_approval" &&
    item.pending_approval?.requested_by !== user?.id

  return (
    <div className="space-y-6 pb-12">
      {/* Back Link & Navigation */}
      <div className="flex items-center justify-between">
        <Link
          href="/"
          className="inline-flex items-center gap-1.5 text-xs font-semibold text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="size-3.5" />
          <span>Back to Work Items</span>
        </Link>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => {
            refetchItem()
            refetchEvents()
            refetchComments()
          }}
          className="gap-1.5 text-xs text-muted-foreground"
        >
          <RefreshCw className="size-3.5" />
          <span>Refresh</span>
        </Button>
      </div>

      {/* Staleness Banner */}
      {isStale && (
        <div className="flex items-center justify-between rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 px-4 text-xs text-amber-800 dark:text-amber-300">
          <div className="flex items-center gap-2.5">
            <AlertTriangle className="size-4 text-amber-600 dark:text-amber-400 shrink-0" />
            <span>
              This item was updated on the server to <strong>version {item.version}</strong> (you are editing v{loadedVersion}). Your draft is preserved.
            </span>
          </div>
          <Button
            size="xs"
            variant="outline"
            onClick={reloadServerVersion}
            className="border-amber-500/40 hover:bg-amber-500/20 text-xs font-semibold ml-3"
          >
            Reload Server Version
          </Button>
        </div>
      )}

      {/* Header: Title, Status, Priority, Assignee, SLA, Version */}
      <div className="rounded-xl border bg-card p-5 shadow-xs space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="space-y-1.5 flex-1 min-w-[280px]">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono text-xs text-muted-foreground">
                {item.id}
              </span>
              <Badge variant="outline" className="font-mono text-[10px] px-1.5 py-0 bg-muted/50">
                v{item.version}
              </Badge>
            </div>
            <h1 className="text-xl font-bold tracking-tight text-foreground sm:text-2xl">
              {item.title}
            </h1>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {getPriorityBadge(item.priority)}
            {getStatusBadge(item.status)}
            {getSlaBadge(item.sla_state)}
          </div>
        </div>

        {/* Metadata Strip */}
        <div className="flex flex-wrap items-center gap-y-2 gap-x-6 border-t pt-3 text-xs text-muted-foreground">
          <div className="flex items-center gap-2">
            <span className="font-medium">Assignee:</span>
            {item.assignee_id ? (
              <div className="flex items-center gap-1.5 font-semibold text-foreground">
                <Avatar className="size-5 border">
                  <AvatarFallback className="text-[9px] bg-primary/10 text-primary font-bold">
                    {item.assignee_name ? item.assignee_name.trim().charAt(0).toUpperCase() : "O"}
                  </AvatarFallback>
                </Avatar>
                <span>{item.assignee_name || item.assignee_id}</span>
              </div>
            ) : (
              <span className="italic text-muted-foreground">Unassigned</span>
            )}
          </div>

          <div>
            <span className="font-medium">Created:</span>{" "}
            <span>{formatRelativeTime(String(item.created_at))}</span>
          </div>

          <div>
            <span className="font-medium">Updated:</span>{" "}
            <span>{formatRelativeTime(String(item.updated_at))}</span>
          </div>

          {item.due_at && (
            <div>
              <span className="font-medium">SLA Due:</span>{" "}
              <span className="font-semibold text-foreground">{formatRelativeTime(String(item.due_at))}</span>
            </div>
          )}
        </div>
      </div>

      {/* Main Grid: Left (Next Actions + Edit Form) | Right (Timeline & Comments) */}
      <div className="grid gap-6 lg:grid-cols-12">
        {/* Left Column (7 cols) */}
        <div className="space-y-6 lg:col-span-7">
          {/* "Next Action" Panel */}
          <Card className="border-primary/20 shadow-xs">
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-semibold flex items-center gap-2">
                  <Sparkles className="size-4 text-primary" />
                  <span>Next Actions</span>
                </CardTitle>
                <Badge variant="outline" className="text-[10px]">
                  State Machine
                </Badge>
              </div>
              <CardDescription className="text-xs">
                Allowed workflow transitions based on team membership and current state.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-wrap items-center gap-2.5">
                {/* Approve / Reject buttons for eligible leads */}
                {isEligibleLead && (
                  <div className="flex items-center gap-2">
                    <Button
                      size="sm"
                      onClick={() => triggerApprovalDecision("approved")}
                      disabled={decideApprovalMutation.isPending}
                      className="bg-emerald-600 hover:bg-emerald-700 text-white font-semibold text-xs gap-1.5 h-8"
                    >
                      {decideApprovalMutation.isPending ? (
                        <Loader2 className="size-3.5 animate-spin" />
                      ) : (
                        <Check className="size-3.5" />
                      )}
                      <span>Approve</span>
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => triggerApprovalDecision("rejected")}
                      disabled={decideApprovalMutation.isPending}
                      className="border-destructive/40 text-destructive hover:bg-destructive/10 font-semibold text-xs gap-1.5 h-8"
                    >
                      <X className="size-3.5" />
                      <span>Reject</span>
                    </Button>
                  </div>
                )}

                {/* Transition Action Buttons */}
                {item.allowed_transitions && item.allowed_transitions.length > 0 ? (
                  item.allowed_transitions.map((target) => (
                    <Button
                      key={target}
                      size="sm"
                      variant={target === "resolved" ? "default" : "secondary"}
                      onClick={() => triggerTransition(target)}
                      disabled={transitionMutation.isPending}
                      className="text-xs font-semibold gap-1.5 h-8"
                    >
                      {transitionMutation.isPending ? (
                        <Loader2 className="size-3.5 animate-spin" />
                      ) : (
                        <ArrowRight className="size-3.5" />
                      )}
                      <span>{formatStatusLabel(target)}</span>
                    </Button>
                  ))
                ) : (
                  <div className="text-xs text-muted-foreground italic py-1">
                    No transitions currently available for your role.
                  </div>
                )}
              </div>
            </CardContent>
          </Card>

          {/* Edit Form with If-Match Optimistic Locking */}
          <Card className="shadow-xs">
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-semibold flex items-center gap-2">
                  <FileEdit className="size-4 text-primary" />
                  <span>Edit Details</span>
                </CardTitle>
                <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                  <span>If-Match:</span>
                  <Badge variant="outline" className="font-mono text-[10px]">
                    v{loadedVersion}
                  </Badge>
                </div>
              </div>
              <CardDescription className="text-xs">
                Modifications require version concurrency match.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={handleSaveForm} className="space-y-4">
                {/* Title */}
                <div className="space-y-1.5">
                  <Label htmlFor="title" className="text-xs font-semibold">
                    Title
                  </Label>
                  <Input
                    id="title"
                    value={draftTitle}
                    onChange={(e) => {
                      setDraftTitle(e.target.value)
                      handleInputChange()
                    }}
                    className="h-9 text-xs"
                    required
                  />
                </div>

                {/* Description */}
                <div className="space-y-1.5">
                  <Label htmlFor="description" className="text-xs font-semibold">
                    Description
                  </Label>
                  <Textarea
                    id="description"
                    rows={4}
                    value={draftDescription}
                    onChange={(e) => {
                      setDraftDescription(e.target.value)
                      handleInputChange()
                    }}
                    className="text-xs resize-y"
                    placeholder="Provide operational context..."
                  />
                </div>

                {/* Priority */}
                <div className="space-y-1.5">
                  <Label htmlFor="priority" className="text-xs font-semibold">
                    Priority
                  </Label>
                  <select
                    id="priority"
                    value={draftPriority}
                    onChange={(e) => {
                      setDraftPriority(Number(e.target.value))
                      handleInputChange()
                    }}
                    className="h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-xs shadow-2xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <option value="1">P1 Critical (1h SLA)</option>
                    <option value="2">P2 High (4h SLA)</option>
                    <option value="3">P3 Medium (24h SLA)</option>
                    <option value="4">P4 Low (72h SLA)</option>
                  </select>
                </div>

                {/* Team Custom Fields Rendered from Schema */}
                {fieldSchemas && fieldSchemas.length > 0 && (
                  <div className="space-y-3 pt-3 border-t">
                    <div className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                      Team Custom Fields
                    </div>
                    <div className="grid gap-3 sm:grid-cols-2">
                      {fieldSchemas.map((schema) => {
                        const val = draftCustomFields[schema.field_key]
                        return (
                          <div key={schema.field_key} className="space-y-1">
                            <Label className="text-xs font-medium">
                              {schema.label} {schema.required && <span className="text-destructive">*</span>}
                            </Label>
                            {schema.type === "number" ? (
                              <Input
                                type="number"
                                value={val !== undefined ? String(val) : ""}
                                onChange={(e) => {
                                  const num = e.target.value === "" ? null : Number(e.target.value)
                                  setDraftCustomFields((prev) => ({
                                    ...prev,
                                    [schema.field_key]: num,
                                  }))
                                  handleInputChange()
                                }}
                                className="h-8 text-xs"
                              />
                            ) : schema.type === "boolean" ? (
                              <div className="flex items-center gap-2 h-8">
                                <input
                                  type="checkbox"
                                  id={`custom-${schema.field_key}`}
                                  checked={!!val}
                                  onChange={(e) => {
                                    setDraftCustomFields((prev) => ({
                                      ...prev,
                                      [schema.field_key]: e.target.checked,
                                    }))
                                    handleInputChange()
                                  }}
                                  className="size-4 rounded border-input"
                                />
                                <label htmlFor={`custom-${schema.field_key}`} className="text-xs">
                                  {schema.label}
                                </label>
                              </div>
                            ) : (
                              <Input
                                value={val !== undefined ? String(val) : ""}
                                onChange={(e) => {
                                  setDraftCustomFields((prev) => ({
                                    ...prev,
                                    [schema.field_key]: e.target.value,
                                  }))
                                  handleInputChange()
                                }}
                                className="h-8 text-xs"
                              />
                            )}
                          </div>
                        )
                      })}
                    </div>
                  </div>
                )}

                {/* Action Buttons */}
                <div className="flex items-center justify-between pt-3 border-t">
                  <div className="text-xs text-muted-foreground">
                    {isFormDirty ? (
                      <span className="text-amber-600 font-medium">Unsaved changes</span>
                    ) : (
                      "No changes pending"
                    )}
                  </div>
                  <Button
                    type="submit"
                    size="sm"
                    disabled={saveMutation.isPending || !isFormDirty}
                    className="font-semibold text-xs gap-1.5 h-8"
                  >
                    {saveMutation.isPending ? (
                      <Loader2 className="size-3.5 animate-spin" />
                    ) : (
                      <Check className="size-3.5" />
                    )}
                    <span>Save Changes</span>
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>
        </div>

        {/* Right Column: Timeline & Comments (5 cols) */}
        <div className="space-y-6 lg:col-span-5">
          <Card className="flex flex-col h-[760px] shadow-xs">
            <CardHeader className="pb-3 border-b">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-semibold flex items-center gap-2">
                  <History className="size-4 text-primary" />
                  <span>Activity & Comments</span>
                </CardTitle>
                <Badge variant="outline" className="text-[10px]">
                  {timelineItems.length} entries
                </Badge>
              </div>
            </CardHeader>

            {/* Scrollable Timeline Stream */}
            <div className="flex-1 overflow-y-auto p-4 space-y-4">
              {timelineItems.length === 0 ? (
                <div className="flex flex-col items-center justify-center p-8 text-center text-muted-foreground">
                  <MessageSquare className="size-8 text-muted-foreground/30 mb-2" />
                  <p className="text-xs">No activity or comments recorded yet.</p>
                </div>
              ) : (
                timelineItems.map((entry) => {
                  const isComment = entry.type === "comment"
                  return (
                    <div
                      key={entry.id}
                      className={`flex gap-3 text-xs ${
                        isComment ? "bg-muted/30 border rounded-lg p-3" : "py-1"
                      }`}
                    >
                      <Avatar className="size-6 border shrink-0 mt-0.5">
                        <AvatarFallback className="text-[9px] font-bold bg-primary/10 text-primary">
                          {entry.authorName ? entry.authorName.trim().charAt(0).toUpperCase() : "U"}
                        </AvatarFallback>
                      </Avatar>

                      <div className="flex-1 min-w-0 space-y-1">
                        <div className="flex items-center justify-between gap-1">
                          <span className="font-semibold text-foreground truncate">
                            {entry.authorName || "User"}
                          </span>
                          <span className="text-[10px] text-muted-foreground shrink-0">
                            {formatRelativeTime(entry.created_at)}
                          </span>
                        </div>

                        <div className="text-foreground/90 font-medium">
                          {entry.title}
                        </div>

                        {entry.subtitle && (
                          <div className="text-[11px] text-muted-foreground italic">
                            {entry.subtitle}
                          </div>
                        )}

                        {entry.body && (
                          <div className="text-xs text-foreground/85 whitespace-pre-wrap mt-1 bg-background/60 p-2 rounded border">
                            {entry.body}
                          </div>
                        )}
                      </div>
                    </div>
                  )
                })
              )}
            </div>

            {/* Comment Input Box with @mention Autocomplete */}
            <div className="border-t p-3 bg-muted/10 relative">
              {filteredMentions.length > 0 && (
                <div className="absolute bottom-full left-3 right-3 mb-1 z-30 max-h-40 overflow-y-auto rounded-lg border bg-popover p-1 shadow-lg">
                  <div className="px-2 py-1 text-[10px] font-semibold text-muted-foreground uppercase">
                    Mention Team Member
                  </div>
                  {filteredMentions.map((m) => (
                    <button
                      key={m.user_id}
                      type="button"
                      onClick={() => insertMention(m)}
                      className="flex w-full items-center justify-between p-1.5 px-2 text-left text-xs rounded hover:bg-accent cursor-pointer"
                    >
                      <div className="flex flex-col">
                        <span className="font-medium text-foreground">{m.name}</span>
                        <span className="text-[10px] text-muted-foreground">{m.email}</span>
                      </div>
                      <Badge variant="outline" className="text-[9px] capitalize">
                        {m.role}
                      </Badge>
                    </button>
                  ))}
                </div>
              )}

              <form onSubmit={handlePostComment} className="space-y-2">
                <Textarea
                  ref={commentInputRef}
                  rows={2}
                  value={commentText}
                  onChange={handleCommentChange}
                  placeholder="Leave a comment (type @ to mention a team member)..."
                  className="text-xs resize-none bg-background"
                />
                <div className="flex items-center justify-between">
                  <span className="text-[10px] text-muted-foreground">
                    Markdown supported · @mentions notified
                  </span>
                  <Button
                    type="submit"
                    size="xs"
                    disabled={commentMutation.isPending || !commentText.trim()}
                    className="gap-1.5 text-xs font-semibold h-7"
                  >
                    {commentMutation.isPending ? (
                      <Loader2 className="size-3 animate-spin" />
                    ) : (
                      <Send className="size-3" />
                    )}
                    <span>Post</span>
                  </Button>
                </div>
              </form>
            </div>
          </Card>
        </div>
      </div>

      {/* --- DIALOG: Transition Reason Prompt --- */}
      <Dialog open={reasonDialogOpen} onOpenChange={setReasonDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-base font-bold">
              Reason Required for Transition
            </DialogTitle>
            <DialogDescription className="text-xs text-muted-foreground">
              Moving to <strong>{pendingTargetState ? formatStatusLabel(pendingTargetState) : ""}</strong> requires an audit justification.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3 py-2">
            <Label htmlFor="transition-reason" className="text-xs font-semibold">
              Reason
            </Label>
            <Textarea
              id="transition-reason"
              rows={3}
              value={transitionReason}
              onChange={(e) => setTransitionReason(e.target.value)}
              placeholder="State the justification or evidence..."
              className="text-xs"
            />
          </div>

          <DialogFooter className="gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setReasonDialogOpen(false)}
              className="text-xs"
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleReasonDialogSubmit}
              disabled={transitionMutation.isPending || !transitionReason.trim()}
              className="text-xs font-semibold"
            >
              {transitionMutation.isPending && <Loader2 className="size-3.5 animate-spin mr-1" />}
              Confirm Transition
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* --- DIALOG: Approval Decision (Approve / Reject) --- */}
      <Dialog open={approvalDialogOpen} onOpenChange={setApprovalDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="text-base font-bold">
              {approvalDecision === "approved" ? "Approve Work Item" : "Reject Work Item"}
            </DialogTitle>
            <DialogDescription className="text-xs text-muted-foreground">
              Provide an operational decision rationale for this sign-off.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3 py-2">
            <Label htmlFor="approval-reason" className="text-xs font-semibold">
              Decision Reason
            </Label>
            <Textarea
              id="approval-reason"
              rows={3}
              value={approvalReason}
              onChange={(e) => setApprovalReason(e.target.value)}
              placeholder="Specify rationale for approval or feedback for rejection..."
              className="text-xs"
            />
          </div>

          <DialogFooter className="gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setApprovalDialogOpen(false)}
              className="text-xs"
            >
              Cancel
            </Button>
            <Button
              size="sm"
              variant={approvalDecision === "approved" ? "default" : "destructive"}
              onClick={handleApprovalDialogSubmit}
              disabled={decideApprovalMutation.isPending || !approvalReason.trim()}
              className="text-xs font-semibold"
            >
              {decideApprovalMutation.isPending && <Loader2 className="size-3.5 animate-spin mr-1" />}
              Submit Decision
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* --- DIALOG: Version Conflict Resolution (409) --- */}
      <Dialog open={conflictDialogOpen} onOpenChange={setConflictDialogOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <div className="flex items-center gap-2 text-destructive">
              <ShieldAlert className="size-5" />
              <DialogTitle className="text-base font-bold">Version Conflict (409)</DialogTitle>
            </div>
            <DialogDescription className="text-xs text-muted-foreground">
              Another user updated this item concurrently. Compare your changes against the server values and choose which to apply.
            </DialogDescription>
          </DialogHeader>

          {conflictServerItem && (
            <div className="space-y-4 py-2">
              {/* Title Field Diff */}
              <div className="rounded-lg border p-3 space-y-2 bg-muted/20">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-bold">Title</span>
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => setConflictFieldSelections((p) => ({ ...p, title: "mine" }))}
                      className={`px-2 py-0.5 text-[10px] rounded font-semibold cursor-pointer ${
                        conflictFieldSelections.title === "mine"
                          ? "bg-primary text-primary-foreground"
                          : "bg-muted text-muted-foreground"
                      }`}
                    >
                      Keep Mine
                    </button>
                    <button
                      type="button"
                      onClick={() => setConflictFieldSelections((p) => ({ ...p, title: "theirs" }))}
                      className={`px-2 py-0.5 text-[10px] rounded font-semibold cursor-pointer ${
                        conflictFieldSelections.title === "theirs"
                          ? "bg-primary text-primary-foreground"
                          : "bg-muted text-muted-foreground"
                      }`}
                    >
                      Use Theirs
                    </button>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div className="p-2 rounded bg-background border">
                    <span className="text-[10px] font-semibold text-muted-foreground block">Your Draft:</span>
                    <span className="font-medium">{draftTitle}</span>
                  </div>
                  <div className="p-2 rounded bg-background border">
                    <span className="text-[10px] font-semibold text-muted-foreground block">Server Value:</span>
                    <span className="font-medium">{conflictServerItem.title}</span>
                  </div>
                </div>
              </div>

              {/* Description Field Diff */}
              <div className="rounded-lg border p-3 space-y-2 bg-muted/20">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-bold">Description</span>
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => setConflictFieldSelections((p) => ({ ...p, description: "mine" }))}
                      className={`px-2 py-0.5 text-[10px] rounded font-semibold cursor-pointer ${
                        conflictFieldSelections.description === "mine"
                          ? "bg-primary text-primary-foreground"
                          : "bg-muted text-muted-foreground"
                      }`}
                    >
                      Keep Mine
                    </button>
                    <button
                      type="button"
                      onClick={() => setConflictFieldSelections((p) => ({ ...p, description: "theirs" }))}
                      className={`px-2 py-0.5 text-[10px] rounded font-semibold cursor-pointer ${
                        conflictFieldSelections.description === "theirs"
                          ? "bg-primary text-primary-foreground"
                          : "bg-muted text-muted-foreground"
                      }`}
                    >
                      Use Theirs
                    </button>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div className="p-2 rounded bg-background border max-h-24 overflow-y-auto">
                    <span className="text-[10px] font-semibold text-muted-foreground block">Your Draft:</span>
                    <span className="text-muted-foreground">{draftDescription || "(empty)"}</span>
                  </div>
                  <div className="p-2 rounded bg-background border max-h-24 overflow-y-auto">
                    <span className="text-[10px] font-semibold text-muted-foreground block">Server Value:</span>
                    <span className="text-muted-foreground">{conflictServerItem.description || "(empty)"}</span>
                  </div>
                </div>
              </div>

              {/* Priority Diff */}
              <div className="rounded-lg border p-3 space-y-2 bg-muted/20">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-bold">Priority</span>
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => setConflictFieldSelections((p) => ({ ...p, priority: "mine" }))}
                      className={`px-2 py-0.5 text-[10px] rounded font-semibold cursor-pointer ${
                        conflictFieldSelections.priority === "mine"
                          ? "bg-primary text-primary-foreground"
                          : "bg-muted text-muted-foreground"
                      }`}
                    >
                      Keep Mine
                    </button>
                    <button
                      type="button"
                      onClick={() => setConflictFieldSelections((p) => ({ ...p, priority: "theirs" }))}
                      className={`px-2 py-0.5 text-[10px] rounded font-semibold cursor-pointer ${
                        conflictFieldSelections.priority === "theirs"
                          ? "bg-primary text-primary-foreground"
                          : "bg-muted text-muted-foreground"
                      }`}
                    >
                      Use Theirs
                    </button>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div className="p-2 rounded bg-background border">
                    <span className="text-[10px] font-semibold text-muted-foreground block">Your Draft:</span>
                    {getPriorityBadge(draftPriority)}
                  </div>
                  <div className="p-2 rounded bg-background border">
                    <span className="text-[10px] font-semibold text-muted-foreground block">Server Value:</span>
                    {getPriorityBadge(conflictServerItem.priority)}
                  </div>
                </div>
              </div>
            </div>
          )}

          <DialogFooter className="gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setConflictDialogOpen(false)}
              className="text-xs"
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleResolveConflictAndSave}
              disabled={saveMutation.isPending}
              className="text-xs font-semibold"
            >
              {saveMutation.isPending && <Loader2 className="size-3.5 animate-spin mr-1" />}
              Apply & Retry Save (v{conflictServerItem?.version})
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

export default function ItemDetailPage() {
  const params = useParams()
  const itemID = params.id as string

  const {
    data: item,
    isLoading,
    isError,
    error,
  } = useQuery<WorkItem>({
    queryKey: queryKeys.items.detail(itemID),
    queryFn: () => apiFetch<WorkItem>(`/items/${itemID}`),
    refetchInterval: 10000,
    refetchOnWindowFocus: true,
    enabled: !!itemID,
  })

  if (isLoading) {
    return (
      <div className="space-y-6 p-4">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-24 w-full" />
        <div className="grid gap-6 md:grid-cols-3">
          <Skeleton className="h-96 md:col-span-2" />
          <Skeleton className="h-96" />
        </div>
      </div>
    )
  }

  if (isError || !item) {
    return (
      <div className="flex flex-col items-center justify-center p-12 text-center">
        <XCircle className="size-12 text-destructive mb-3" />
        <h2 className="text-xl font-bold">Failed to load work item</h2>
        <p className="text-sm text-muted-foreground mt-1 max-w-md">
          {error instanceof Error ? error.message : "Item not found or you lack permission to view it."}
        </p>
        <Link href="/">
          <Button variant="outline" className="mt-4 gap-2">
            <ArrowLeft className="size-4" />
            <span>Return to Dashboard</span>
          </Button>
        </Link>
      </div>
    )
  }

  return <ItemDetailContent key={item.id} initialItem={item} itemID={itemID} />
}
