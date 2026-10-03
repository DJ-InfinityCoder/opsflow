"use client"

import * as React from "react"
import { useQuery } from "@tanstack/react-query"
import { useAuth } from "@/providers/auth-provider"
import { apiFetch, ApiError } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { AnalyticsSummary } from "@/types"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
} from "recharts"
import {
  ShieldAlert,
  Clock,
  AlertTriangle,
  TrendingUp,
  BarChart3,
  Loader2,
  RefreshCw,
  Building2,
} from "lucide-react"

export default function AnalyticsPage() {
  const { isLead, isSystemAdmin, memberships, activeTeam, currentRole } = useAuth()

  // Eligible teams (user is lead or user is admin)
  const eligibleTeams = React.useMemo(() => {
    if (isSystemAdmin) {
      return memberships
    }
    return memberships.filter((m) => m.role === "lead")
  }, [memberships, isSystemAdmin])

  const [selectedTeamId, setSelectedTeamId] = React.useState<string>("")
  const effectiveTeamId =
    selectedTeamId || activeTeam?.team_id || (eligibleTeams[0]?.team_id ?? "")

  const selectedTeamName =
    memberships.find((m) => m.team_id === effectiveTeamId)?.team_name || "Team"

  // Fetch summary from GET /analytics/summary?team=:id
  const {
    data: summary,
    isLoading,
    isError,
    error,
    refetch,
    isRefetching,
  } = useQuery<AnalyticsSummary>({
    queryKey: queryKeys.analytics.summary(effectiveTeamId),
    queryFn: () => apiFetch<AnalyticsSummary>(`/analytics/summary?team=${effectiveTeamId}`),
    enabled: !!effectiveTeamId && (isLead || isSystemAdmin),
    staleTime: 30_000,
  })

  // Permission Guard
  if (!isLead && !isSystemAdmin) {
    return (
      <div className="flex flex-col items-center justify-center p-12 text-center max-w-md mx-auto my-12">
        <div className="rounded-full bg-destructive/10 p-3 mb-4">
          <ShieldAlert className="size-10 text-destructive" />
        </div>
        <h2 className="text-xl font-bold">Access Restricted</h2>
        <p className="text-sm text-muted-foreground mt-2">
          Operations Analytics is reserved for team leads and system administrators. Your current role is{" "}
          <span className="capitalize font-semibold text-foreground">{currentRole || "operator"}</span>.
        </p>
      </div>
    )
  }

  // Format MTTR (seconds -> hours/mins)
  const formatMTTR = (seconds: number) => {
    if (!seconds || seconds <= 0) return "N/A"
    const hours = seconds / 3600
    if (hours < 1) {
      const mins = Math.round(seconds / 60)
      return `${mins}m`
    }
    return `${hours.toFixed(1)}h`
  }

  // Chart data: Open items by status
  const statusChartData = summary?.by_status
    ? [
        { name: "New", key: "new", count: summary.by_status["new"] || 0, fill: "#3b82f6" },
        { name: "Triaged", key: "triaged", count: summary.by_status["triaged"] || 0, fill: "#6366f1" },
        { name: "In Progress", key: "in_progress", count: summary.by_status["in_progress"] || 0, fill: "#eab308" },
        { name: "Pending Approval", key: "pending_approval", count: summary.by_status["pending_approval"] || 0, fill: "#f97316" },
        { name: "Resolved", key: "resolved", count: summary.by_status["resolved"] || 0, fill: "#10b981" },
        { name: "Closed", key: "closed", count: summary.by_status["closed"] || 0, fill: "#64748b" },
      ]
    : []

  // Chart data: Priority breakdown
  const priorityChartData = summary?.by_priority
    ? [
        { name: "P1 Critical", count: summary.by_priority[1] || 0, fill: "#ef4444" },
        { name: "P2 High", count: summary.by_priority[2] || 0, fill: "#f97316" },
        { name: "P3 Medium", count: summary.by_priority[3] || 0, fill: "#3b82f6" },
        { name: "P4 Low", count: summary.by_priority[4] || 0, fill: "#64748b" },
      ]
    : []

  // Chart data: SLA Health
  const slaChartData = summary
    ? [
        {
          name: "SLA Breached",
          count: summary.sla_breached_count,
          fill: "#ef4444",
        },
        {
          name: "SLA Warning (<4h)",
          count: summary.sla_warning_count,
          fill: "#f59e0b",
        },
        {
          name: "Within SLA",
          count: Math.max(
            0,
            summary.open_items - summary.sla_breached_count - summary.sla_warning_count
          ),
          fill: "#10b981",
        },
      ]
    : []

  return (
    <div className="space-y-6">
      {/* Header and Team Selector */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight text-foreground sm:text-3xl">
              Operations Analytics
            </h1>
            <Badge variant="default" className="text-xs">
              {isSystemAdmin ? "System Admin" : "Lead Authorization"}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground mt-0.5">
            Operational velocity, SLA health, and workflow bottlenecks for {selectedTeamName}.
          </p>
        </div>

        <div className="flex items-center gap-2">
          {eligibleTeams.length > 1 && (
            <Select
              value={effectiveTeamId}
              onValueChange={(val) => {
                if (val) setSelectedTeamId(val)
              }}
            >
              <SelectTrigger className="w-52 h-9 text-xs">
                <Building2 className="size-3.5 mr-1 text-muted-foreground" />
                <SelectValue placeholder="Select team" />
              </SelectTrigger>
              <SelectContent>
                {eligibleTeams.map((m) => (
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

      {isLoading ? (
        <div className="flex flex-col items-center justify-center p-16 text-sm text-muted-foreground gap-2">
          <Loader2 className="size-6 animate-spin text-primary" />
          <span>Loading analytics metrics...</span>
        </div>
      ) : isError ? (
        <Card className="border-destructive/40 bg-destructive/5">
          <CardContent className="p-6 text-center">
            <ShieldAlert className="size-8 text-destructive mx-auto mb-2" />
            <p className="text-sm font-semibold text-destructive">
              {error instanceof ApiError && error.status === 403
                ? "You do not have lead authorization for this team's analytics."
                : "Failed to load team analytics metrics."}
            </p>
          </CardContent>
        </Card>
      ) : summary ? (
        <>
          {/* Key Metric Cards */}
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                  Total Items
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{summary.total_items}</div>
                <p className="text-[11px] text-muted-foreground mt-0.5">All-time tracked</p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                  Open Items
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold text-primary">{summary.open_items}</div>
                <p className="text-[11px] text-muted-foreground mt-0.5">Active in queue</p>
              </CardContent>
            </Card>

            <Card className={summary.sla_breached_count > 0 ? "border-red-500/40" : ""}>
              <CardHeader className="pb-2">
                <CardTitle className="text-xs font-semibold text-destructive uppercase tracking-wider flex items-center gap-1">
                  <AlertTriangle className="size-3.5" />
                  SLA Breached
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold text-destructive">
                  {summary.sla_breached_count}
                </div>
                <p className="text-[11px] text-muted-foreground mt-0.5">Exceeded target SLA</p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-xs font-semibold text-amber-500 uppercase tracking-wider flex items-center gap-1">
                  <Clock className="size-3.5" />
                  SLA Warning
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold text-amber-500">
                  {summary.sla_warning_count}
                </div>
                <p className="text-[11px] text-muted-foreground mt-0.5">Due in under 4 hours</p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-1">
                  <TrendingUp className="size-3.5" />
                  Avg MTTR
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold text-emerald-500">
                  {formatMTTR(summary.mttr_seconds)}
                </div>
                <p className="text-[11px] text-muted-foreground mt-0.5">Mean time to resolve</p>
              </CardContent>
            </Card>
          </div>

          {/* Visual Charts: Open by Status & SLA Health */}
          <div className="grid gap-6 lg:grid-cols-2">
            {/* Status Breakdown Bar Chart */}
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-semibold flex items-center gap-2">
                  <BarChart3 className="size-4 text-primary" />
                  Items by Status
                </CardTitle>
                <CardDescription className="text-xs">
                  Active distribution across the state machine
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="h-64 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart
                      data={statusChartData}
                      margin={{ top: 10, right: 10, left: -20, bottom: 20 }}
                    >
                      <CartesianGrid strokeDasharray="3 3" opacity={0.2} vertical={false} />
                      <XAxis
                        dataKey="name"
                        fontSize={11}
                        interval={0}
                        angle={-15}
                        textAnchor="end"
                      />
                      <YAxis allowDecimals={false} fontSize={11} />
                      <Tooltip
                        contentStyle={{
                          backgroundColor: "hsl(var(--card))",
                          borderColor: "hsl(var(--border))",
                          borderRadius: "8px",
                          fontSize: "12px",
                        }}
                      />
                      <Bar dataKey="count" radius={[4, 4, 0, 0]}>
                        {statusChartData.map((entry, index) => (
                          <Cell key={`cell-${index}`} fill={entry.fill} />
                        ))}
                      </Bar>
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </CardContent>
            </Card>

            {/* SLA Health / Aging Buckets */}
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-semibold flex items-center gap-2">
                  <Clock className="size-4 text-amber-500" />
                  SLA Compliance & Aging Buckets
                </CardTitle>
                <CardDescription className="text-xs">
                  Proportion of open tickets meeting target turnaround times
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="h-64 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart
                      data={slaChartData}
                      margin={{ top: 10, right: 10, left: -20, bottom: 10 }}
                    >
                      <CartesianGrid strokeDasharray="3 3" opacity={0.2} vertical={false} />
                      <XAxis dataKey="name" fontSize={11} />
                      <YAxis allowDecimals={false} fontSize={11} />
                      <Tooltip
                        contentStyle={{
                          backgroundColor: "hsl(var(--card))",
                          borderColor: "hsl(var(--border))",
                          borderRadius: "8px",
                          fontSize: "12px",
                        }}
                      />
                      <Bar dataKey="count" radius={[4, 4, 0, 0]}>
                        {slaChartData.map((entry, index) => (
                          <Cell key={`sla-cell-${index}`} fill={entry.fill} />
                        ))}
                      </Bar>
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Priority Distribution */}
          <Card>
            <CardHeader>
              <CardTitle className="text-sm font-semibold flex items-center gap-2">
                <AlertTriangle className="size-4 text-red-500" />
                Active Items by Priority
              </CardTitle>
              <CardDescription className="text-xs">
                Open workload prioritized from P1 (Critical) to P4 (Low)
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="h-56 w-full">
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart
                    data={priorityChartData}
                    margin={{ top: 10, right: 10, left: -20, bottom: 10 }}
                  >
                    <CartesianGrid strokeDasharray="3 3" opacity={0.2} vertical={false} />
                    <XAxis dataKey="name" fontSize={11} />
                    <YAxis allowDecimals={false} fontSize={11} />
                    <Tooltip
                      contentStyle={{
                        backgroundColor: "hsl(var(--card))",
                        borderColor: "hsl(var(--border))",
                        borderRadius: "8px",
                        fontSize: "12px",
                      }}
                    />
                    <Bar dataKey="count" radius={[4, 4, 0, 0]}>
                      {priorityChartData.map((entry, index) => (
                        <Cell key={`priority-cell-${index}`} fill={entry.fill} />
                      ))}
                    </Bar>
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </CardContent>
          </Card>
        </>
      ) : null}
    </div>
  )
}
