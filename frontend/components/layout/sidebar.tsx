"use client"

import * as React from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { useAuth } from "@/providers/auth-provider"
import { cn } from "@/lib/utils"
import {
  LayoutDashboard,
  CheckCircle2,
  Bell,
  BarChart3,
  Layers,
  ShieldAlert,
  Activity,
  Cpu,
} from "lucide-react"

export function Sidebar() {
  const pathname = usePathname()
  const { isLead, isSystemAdmin, currentRole, activeTeam } = useAuth()

  const navItems = [
    {
      title: "Dashboard",
      href: "/",
      icon: LayoutDashboard,
      show: true,
    },
    {
      title: "My Work",
      href: "/my-work",
      icon: CheckCircle2,
      show: true,
    },
    {
      title: "Activity Feed",
      href: "/feed",
      icon: Activity,
      show: true,
    },
    {
      title: "Notifications",
      href: "/notifications",
      icon: Bell,
      show: true,
    },
    {
      title: "Analytics",
      href: "/analytics",
      icon: BarChart3,
      show: isLead || isSystemAdmin,
      badge: "Lead",
    },
    {
      title: "Failed Jobs",
      href: "/admin/jobs",
      icon: Cpu,
      show: isSystemAdmin,
      badge: "Admin",
    },
  ]

  return (
    <aside className="hidden md:flex h-screen w-64 flex-col border-r bg-card/60 backdrop-blur-md">
      {/* Brand Header */}
      <div className="flex h-14 items-center gap-2.5 border-b px-4">
        <div className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
          <Layers className="size-4.5" />
        </div>
        <div className="flex flex-col">
          <span className="font-bold text-base tracking-tight text-foreground">
            OpsFlow
          </span>
          <span className="text-[10px] text-muted-foreground uppercase tracking-widest font-mono">
            Work Tracker
          </span>
        </div>
      </div>

      {/* Navigation */}
      <div className="flex-1 overflow-y-auto py-4 px-3">
        <div className="mb-2 px-3 text-[11px] font-semibold text-muted-foreground uppercase tracking-wider">
          Operations
        </div>
        <nav className="space-y-1">
          {navItems
            .filter((item) => item.show)
            .map((item) => {
              const Icon = item.icon
              const isActive =
                item.href === "/"
                  ? pathname === "/"
                  : pathname.startsWith(item.href)

              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    "group flex items-center justify-between rounded-md px-3 py-2 text-sm font-medium transition-colors",
                    isActive
                      ? "bg-primary text-primary-foreground shadow-xs font-semibold"
                      : "text-muted-foreground hover:bg-accent hover:text-foreground"
                  )}
                >
                  <div className="flex items-center gap-2.5">
                    <Icon
                      className={cn(
                        "size-4 shrink-0 transition-colors",
                        isActive
                          ? "text-primary-foreground"
                          : "text-muted-foreground group-hover:text-foreground"
                      )}
                    />
                    <span>{item.title}</span>
                  </div>
                  {item.badge && !isActive && (
                    <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                      {item.badge}
                    </span>
                  )}
                </Link>
              )
            })}
        </nav>
      </div>

      {/* Active Team / Role Footer */}
      <div className="border-t p-3 bg-muted/30">
        <div className="flex flex-col gap-1 rounded-lg border bg-background/80 p-2.5 shadow-2xs">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold truncate">
              {activeTeam?.team_name || "No Team"}
            </span>
            {isSystemAdmin ? (
              <span className="flex items-center gap-1 text-[10px] font-medium text-destructive">
                <ShieldAlert className="size-3" />
                Admin
              </span>
            ) : (
              <span className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
                {currentRole || "No Role"}
              </span>
            )}
          </div>
          <span className="text-[11px] text-muted-foreground truncate">
            {isLead
              ? "Lead authorization enabled"
              : currentRole === "operator"
              ? "Operator permissions active"
              : "Reporter permissions active"}
          </span>
        </div>
      </div>
    </aside>
  )
}
