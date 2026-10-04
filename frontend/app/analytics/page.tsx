"use client"

import * as React from "react"
import { useQuery } from "@tanstack/react-query"
import { useAuth } from "@/providers/auth-provider"
import { apiFetch, ApiError } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { AnalyticsSummary, AnalyticsTeamMetric } from "@/types"
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
  const { isSystemAdmin, memberships, activeTeam, currentRole } = useAuth()
  const canRequestAnalytics = isSystemAdmin || memberships.some((membership) => membership.role === "lead")

  const {
    data: eligibleTeams = [],
    isLoading: isLoadingTeams,
    isError: isTeamsError,
  } = useQuery<AnalyticsTeamMetric[]>({
    queryKey: queryKeys.analytics.teams(),
    queryFn: () => apiFetch<AnalyticsTeamMetric[]>("/analytics/teams"),
    enabled: canRequestAnalytics,
    staleTime: 30_000,
  })

  const [selectedTeamId, setSelectedTeamId] = React.useState<string>("")
  const defaultTeamId = eligibleTeams.some((team) => team.team_id === activeTeam?.team_id)
    ? activeTeam?.team_id ?? ""
    : eligibleTeams[0]?.team_id ?? ""
  const effectiveTeamId = selectedTeamId || defaultTeamId

  const selectedTeamName =
    eligibleTeams.find((team) => team.team_id === effectiveTeamId)?.team_name || "Team"

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
    enabled: !!effectiveTeamId && canRequestAnalytics,
    staleTime: 30_000,
  })

  // Permission Guard
  if (!canRequestAnalytics) {
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

  // Chart data: Open item age buckets
  const agingChartData = summary
    ? [
        { name: "< 1 day", count: summary.aging_buckets.under_1_day || 0, fill: "#14b8a6" },
        { name: "1-3 days", count: summary.aging_buckets["1_to_3_days"] || 0, fill: "#0ea5e9" },
        { name: "3-7 days", count: summary.aging_buckets["3_to_7_days"] || 0, fill: "#f59e0b" },
        { name: "> 7 days", count: summary.aging_buckets.over_7_days || 0, fill: "#ef4444" },
      ]
    : []
  const teamSLABreachData = eligibleTeams.map((team) => ({
    name: team.team_name,
    count: team.sla_breached_count,
    fill: team.sla_breached_count > 0 ? "#ef4444" : "#14b8a6",
  }))

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
                <SelectValue placeholder="Select team">{selectedTeamName}</SelectValue>
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

      {isLoadingTeams ? (
        <div className="flex items-center justify-center p-16 text-sm text-muted-foreground gap-2">
          <Loader2 className="size-6 animate-spin text-primary" />
          <span>Loading authorized teams...</span>
        </div>
      ) : isTeamsError ? (
        <Card className="border-destructive/40 bg-destructive/5">
          <CardContent className="p-6 text-center text-sm text-destructive">
            Failed to load analytics teams.
          </CardContent>
        </Card>
      ) : eligibleTeams.length === 0 ? (
        <Card>
          <CardContent className="p-6 text-center text-sm text-muted-foreground">
            No teams are available for analytics.
          </CardContent>
        </Card>
      ) : isLoading ? (
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

          {/* Open workload and aging charts */}
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

            {/* Open-item aging buckets */}
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-semibold flex items-center gap-2">
                  <Clock className="size-4 text-amber-500" />
                  Open Items by Age
                </CardTitle>
                <CardDescription className="text-xs">
                  Time since creation for unresolved work items
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="h-64 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart
                      data={agingChartData}
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
                        {agingChartData.map((entry, index) => (
                          <Cell key={`age-cell-${index}`} fill={entry.fill} />
                        ))}
                      </Bar>
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="text-sm font-semibold flex items-center gap-2">
                <AlertTriangle className="size-4 text-destructive" />
                Breached SLA by Team
              </CardTitle>
              <CardDescription className="text-xs">
                Open items past their target SLA across teams you are authorized to view
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="h-64 w-full">
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart data={teamSLABreachData} margin={{ top: 10, right: 10, left: -20, bottom: 20 }}>
                    <CartesianGrid strokeDasharray="3 3" opacity={0.2} vertical={false} />
                    <XAxis dataKey="name" fontSize={11} interval={0} angle={-15} textAnchor="end" />
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
                      {teamSLABreachData.map((entry) => (
                        <Cell key={entry.name} fill={entry.fill} />
                      ))}
                    </Bar>
                  </BarChart>
                </ResponsiveContainer>
              </div>
            </CardContent>
          </Card>

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
