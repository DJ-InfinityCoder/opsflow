"use client"

import * as React from "react"
import { useQuery } from "@tanstack/react-query"
import { useAuth } from "@/providers/auth-provider"
import { apiFetch, ApiError } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { AdminJobsPage } from "@/types"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  ShieldAlert,
  AlertTriangle,
  RefreshCw,
  Loader2,
  Clock,
  CheckCircle2,
  ChevronRight,
  Code,
  Copy,
  Check,
  Cpu,
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

export default function AdminFailedJobsPage() {
  const { isSystemAdmin } = useAuth()
  const [statusFilter, setStatusFilter] = React.useState<string>("failed")
  const [cursor, setCursor] = React.useState<number | undefined>(undefined)
  const [copiedId, setCopiedId] = React.useState<number | null>(null)
  const [expandedPayloads, setExpandedPayloads] = React.useState<Record<number, boolean>>({})

  // Query outbox jobs
  const {
    data,
    isLoading,
    isError,
    error,
    refetch,
    isRefetching,
  } = useQuery<AdminJobsPage>({
    queryKey: [...queryKeys.admin.jobs(statusFilter), { cursor }],
    queryFn: () => {
      const cursorParam = cursor ? `&cursor=${cursor}` : ""
      return apiFetch<AdminJobsPage>(`/admin/jobs?status=${statusFilter}&limit=25${cursorParam}`)
    },
    enabled: isSystemAdmin,
    staleTime: 10_000,
  })

  const jobs = data?.jobs || []
  const nextCursor = data?.next_cursor

  // Access check
  if (!isSystemAdmin) {
    return (
      <div className="flex flex-col items-center justify-center p-12 text-center max-w-md mx-auto my-12">
        <div className="rounded-full bg-destructive/10 p-3 mb-4">
          <ShieldAlert className="size-10 text-destructive" />
        </div>
        <h2 className="text-xl font-bold">Access Restricted</h2>
        <p className="text-sm text-muted-foreground mt-2">
          The background job queue and dead-letter inspector are restricted exclusively to system administrators.
        </p>
      </div>
    )
  }

  const copyToClipboard = (text: string, id: number) => {
    navigator.clipboard.writeText(text)
    setCopiedId(id)
    toast.success("Error copied to clipboard")
    setTimeout(() => setCopiedId(null), 2000)
  }

  const togglePayload = (id: number) => {
    setExpandedPayloads((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "failed":
        return <Badge variant="destructive">Failed (Dead-Letter)</Badge>
      case "pending":
        return (
          <Badge variant="outline" className="text-amber-500 border-amber-500/40">
            Pending
          </Badge>
        )
      case "completed":
        return (
          <Badge variant="outline" className="text-emerald-500 border-emerald-500/40">
            Completed
          </Badge>
        )
      default:
        return <Badge variant="secondary">{status}</Badge>
    }
  }

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      {/* Page Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight text-foreground sm:text-3xl">
              Failed Outbox Jobs
            </h1>
            <Badge variant="destructive" className="text-xs">
              System Admin
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground mt-0.5">
            Dead-lettered asynchronous queue jobs with error stack traces and retry attempts.
          </p>
        </div>

        <div className="flex items-center gap-2">
          <Tabs
            value={statusFilter}
            onValueChange={(val) => {
              setStatusFilter(val)
              setCursor(undefined)
            }}
          >
            <TabsList className="h-9">
              <TabsTrigger value="failed" className="text-xs px-3">
                Failed
              </TabsTrigger>
              <TabsTrigger value="pending" className="text-xs px-3">
                Pending
              </TabsTrigger>
              <TabsTrigger value="completed" className="text-xs px-3">
                Completed
              </TabsTrigger>
            </TabsList>
          </Tabs>

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
              <Cpu className="size-4 text-primary" />
              Outbox Queue ({statusFilter})
            </CardTitle>
            <Badge variant="outline" className="text-xs">
              {jobs.length} jobs shown
            </Badge>
          </div>
          <CardDescription className="text-xs">
            Asynchronous workers claim tasks with FOR UPDATE SKIP LOCKED and dead-letter on max attempts.
          </CardDescription>
        </CardHeader>

        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex items-center justify-center p-16 text-sm text-muted-foreground gap-2">
              <Loader2 className="size-5 animate-spin text-primary" />
              Loading outbox queue...
            </div>
          ) : isError ? (
            <div className="flex flex-col items-center justify-center p-12 text-center text-destructive">
              <AlertTriangle className="size-10 mb-2" />
              <p className="font-semibold text-sm">Failed to load outbox jobs</p>
              <p className="text-xs text-muted-foreground mt-1">
                {error instanceof ApiError ? error.message : "Network error"}
              </p>
            </div>
          ) : jobs.length === 0 ? (
            <div className="flex flex-col items-center justify-center p-16 text-center text-muted-foreground">
              <CheckCircle2 className="size-10 text-emerald-500 mb-3" />
              <p className="text-sm font-semibold text-foreground">
                No {statusFilter} outbox jobs
              </p>
              <p className="text-xs text-muted-foreground mt-1 max-w-sm">
                The asynchronous worker queue is clean and operating smoothly.
              </p>
            </div>
          ) : (
            <div className="divide-y">
              {jobs.map((job) => (
                <div key={job.id} className="p-4 space-y-3 transition-colors hover:bg-muted/30">
                  {/* Top Bar: ID, Type, Status, Attempts */}
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2.5">
                      <span className="font-mono text-xs font-bold text-muted-foreground">
                        #{job.id}
                      </span>
                      <Badge variant="outline" className="font-mono text-[11px] font-semibold">
                        {job.type}
                      </Badge>
                      {getStatusBadge(job.status)}
                    </div>

                    <div className="flex items-center gap-3 text-xs text-muted-foreground">
                      <span className="flex items-center gap-1">
                        <Clock className="size-3" />
                        Created {formatRelative(job.created_at)}
                      </span>
                      <span>•</span>
                      <span
                        className={`font-semibold ${
                          job.attempts >= job.max_attempts
                            ? "text-destructive"
                            : "text-foreground"
                        }`}
                      >
                        Attempts: {job.attempts}/{job.max_attempts}
                      </span>
                    </div>
                  </div>

                  {/* Dedupe Key */}
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <span className="font-semibold text-[11px] uppercase tracking-wider">
                      Dedupe Key:
                    </span>
                    <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-foreground">
                      {job.dedupe_key}
                    </code>
                  </div>

                  {/* Last Error (Dead-letter reason) */}
                  {job.last_error && (
                    <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 space-y-1.5">
                      <div className="flex items-center justify-between">
                        <span className="text-[11px] font-bold text-destructive uppercase tracking-wider flex items-center gap-1.5">
                          <AlertTriangle className="size-3.5" />
                          Last Error
                        </span>
                        <Button
                          variant="ghost"
                          size="xs"
                          onClick={() => copyToClipboard(job.last_error || "", job.id)}
                          className="h-6 px-1.5 text-[10px] text-destructive hover:bg-destructive/10"
                        >
                          {copiedId === job.id ? (
                            <>
                              <Check className="size-3 mr-1" />
                              Copied
                            </>
                          ) : (
                            <>
                              <Copy className="size-3 mr-1" />
                              Copy Error
                            </>
                          )}
                        </Button>
                      </div>
                      <pre className="text-xs font-mono text-destructive whitespace-pre-wrap break-all bg-background/50 p-2 rounded border border-destructive/20 overflow-x-auto">
                        {job.last_error}
                      </pre>
                    </div>
                  )}

                  {/* Payload toggle */}
                  <div>
                    <Button
                      variant="ghost"
                      size="xs"
                      onClick={() => togglePayload(job.id)}
                      className="text-[11px] text-muted-foreground hover:text-foreground h-6 px-1.5 gap-1"
                    >
                      <Code className="size-3" />
                      {expandedPayloads[job.id] ? "Hide Payload" : "View Payload JSON"}
                    </Button>
                    {expandedPayloads[job.id] && (
                      <pre className="mt-2 text-[11px] font-mono bg-muted/80 p-2.5 rounded border overflow-x-auto">
                        {JSON.stringify(job.payload, null, 2)}
                      </pre>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* Next Cursor Pagination */}
          {nextCursor && (
            <div className="p-3 border-t text-center bg-muted/20">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setCursor(nextCursor)}
                className="text-xs font-semibold gap-1"
              >
                Next Page
                <ChevronRight className="size-3.5" />
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
