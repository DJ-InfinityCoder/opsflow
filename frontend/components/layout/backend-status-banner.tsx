"use client"

import * as React from "react"
import { useQueryClient } from "@tanstack/react-query"
import { getApiBaseUrl, pingBackendHealth } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { toast } from "sonner"
import {
  AlertTriangle,
  ExternalLink,
  Loader2,
  RefreshCw,
  X,
  CheckCircle2,
  ServerOff,
} from "lucide-react"

export function BackendStatusBanner() {
  const queryClient = useQueryClient()
  const [status, setStatus] = React.useState<"unknown" | "online" | "sleeping" | "waking">("unknown")
  const [wakeSeconds, setWakeSeconds] = React.useState(0)
  const [dismissed, setDismissed] = React.useState(false)

  const baseUrl = React.useMemo(() => getApiBaseUrl(), [])
  const healthzUrl = React.useMemo(() => `${baseUrl.replace(/\/+$/, "")}/healthz`, [baseUrl])

  // Check health on mount and every 45s
  React.useEffect(() => {
    let mounted = true

    const check = async () => {
      const ok = await pingBackendHealth(5000)
      if (!mounted) return
      if (ok) {
        setStatus((prev) => (prev === "waking" ? "online" : prev === "sleeping" ? "online" : "online"))
      } else {
        setStatus((prev) => (prev === "waking" ? "waking" : "sleeping"))
      }
    }

    check()
    const interval = setInterval(check, 45000)
    return () => {
      mounted = false
      clearInterval(interval)
    }
  }, [])

  // Active polling when waking up
  React.useEffect(() => {
    if (status !== "waking") return

    let elapsed = 0
    setWakeSeconds(0)

    const timer = setInterval(() => {
      elapsed += 1
      setWakeSeconds(elapsed)
    }, 1000)

    const poller = setInterval(async () => {
      const ok = await pingBackendHealth(4000)
      if (ok) {
        setStatus("online")
        clearInterval(timer)
        clearInterval(poller)
        toast.success("Backend is awake and connected!")
        // Refresh all application queries
        queryClient.invalidateQueries()
      } else if (elapsed > 70) {
        setStatus("sleeping")
        clearInterval(timer)
        clearInterval(poller)
        toast.error("Wake-up took longer than 70s. Try clicking 'Open Direct Link'.")
      }
    }, 3000)

    return () => {
      clearInterval(timer)
      clearInterval(poller)
    }
  }, [status, queryClient])

  const handleWakeUp = () => {
    setStatus("waking")
    setDismissed(false)
    toast.info("Sending wake-up ping to backend...")
    // Fire an immediate un-awaited ping to wake Render
    pingBackendHealth(10000).catch(() => {})
  }

  // If online or unknown, don't show the banner
  if (status === "online" || status === "unknown" || dismissed) {
    return null
  }

  return (
    <div className="w-full bg-amber-500/10 border-b border-amber-500/20 px-4 py-2.5 text-xs text-foreground transition-all">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 max-w-7xl mx-auto">
        <div className="flex items-center gap-2.5">
          <div className="flex size-7 items-center justify-center rounded-sm bg-amber-500/20 text-amber-600 dark:text-amber-400 shrink-0">
            {status === "waking" ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <ServerOff className="size-4" />
            )}
          </div>
          <div>
            <div className="flex items-center gap-2">
              <span className="font-semibold text-amber-700 dark:text-amber-300">
                {status === "waking"
                  ? `Waking up backend server... (${wakeSeconds}s)`
                  : "Backend Server is Sleeping / Starting Up"}
              </span>
              <Badge variant="outline" className="text-[10px] px-1.5 py-0 h-4 border-amber-500/30 text-amber-700 dark:text-amber-300">
                Render Free Tier
              </Badge>
            </div>
            <p className="text-[11px] text-muted-foreground mt-0.5">
              {status === "waking"
                ? "Connecting to Render. Cold starts take ~40–50 seconds on the free tier."
                : "Free instances sleep after 15m of inactivity. Click Wake Server to start it."}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          {status !== "waking" ? (
            <Button
              size="sm"
              onClick={handleWakeUp}
              className="h-7 px-2.5 text-xs font-medium gap-1.5 bg-amber-600 hover:bg-amber-700 text-white rounded-sm shadow-none"
            >
              <RefreshCw className="size-3" />
              <span>Wake Up Backend</span>
            </Button>
          ) : (
            <Button
              size="sm"
              disabled
              className="h-7 px-2.5 text-xs font-medium gap-1.5 bg-amber-600/70 text-white rounded-sm shadow-none"
            >
              <Loader2 className="size-3 animate-spin" />
              <span>Waking Up...</span>
            </Button>
          )}

          <a
            href={healthzUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center justify-center h-7 px-2.5 text-xs font-normal gap-1 border border-amber-500/30 hover:bg-amber-500/10 text-amber-800 dark:text-amber-200 rounded-sm shadow-none transition-colors"
          >
            <span>Direct Link</span>
            <ExternalLink className="size-3" />
          </a>

          <Button
            size="icon-xs"
            variant="ghost"
            onClick={() => setDismissed(true)}
            className="size-7 text-muted-foreground hover:text-foreground"
            title="Dismiss notice"
          >
            <X className="size-3.5" />
          </Button>
        </div>
      </div>
    </div>
  )
}

export function BackendStatusIndicator() {
  const [status, setStatus] = React.useState<"unknown" | "online" | "sleeping" | "waking">("unknown")
  const baseUrl = React.useMemo(() => getApiBaseUrl(), [])
  const healthzUrl = React.useMemo(() => `${baseUrl.replace(/\/+$/, "")}/healthz`, [baseUrl])

  React.useEffect(() => {
    let mounted = true
    const check = async () => {
      const ok = await pingBackendHealth(5000)
      if (!mounted) return
      setStatus(ok ? "online" : "sleeping")
    }
    check()
    const interval = setInterval(check, 60000)
    return () => {
      mounted = false
      clearInterval(interval)
    }
  }, [])

  const handleWake = async () => {
    setStatus("waking")
    toast.info("Pinging backend to wake from sleep...")
    const ok = await pingBackendHealth(15000)
    setStatus(ok ? "online" : "sleeping")
    if (ok) {
      toast.success("Backend is awake!")
    } else {
      toast.error("Backend still starting up. Try opening the direct link.")
    }
  }

  if (status === "online") {
    return (
      <div
        className="hidden md:flex items-center gap-1.5 px-2 py-1 text-[11px] text-muted-foreground font-medium rounded-sm hover:bg-muted/50 cursor-default"
        title="Backend API connected & healthy"
      >
        <span className="size-2 rounded-full bg-emerald-500 inline-block animate-pulse" />
        <span className="text-[10px]">API Online</span>
      </div>
    )
  }

  if (status === "waking") {
    return (
      <div
        className="flex items-center gap-1.5 px-2 py-1 text-[11px] text-amber-600 dark:text-amber-400 font-medium rounded-sm bg-amber-500/10 cursor-pointer"
        onClick={handleWake}
        title="Waking backend..."
      >
        <Loader2 className="size-3 animate-spin text-amber-500" />
        <span className="text-[10px]">Waking API...</span>
      </div>
    )
  }

  return (
    <button
      onClick={handleWake}
      className="flex items-center gap-1.5 px-2 py-1 text-[11px] text-amber-600 dark:text-amber-400 font-medium rounded-sm border border-amber-500/30 bg-amber-500/10 hover:bg-amber-500/20 transition-colors cursor-pointer"
      title="Backend appears to be sleeping. Click to wake."
    >
      <span className="size-2 rounded-full bg-amber-500 inline-block animate-ping" />
      <span className="text-[10px]">Wake Backend</span>
    </button>
  )
}
