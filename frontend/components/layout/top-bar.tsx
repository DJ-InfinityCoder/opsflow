"use client"

import * as React from "react"
import { useAuth } from "@/providers/auth-provider"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  ChevronDown,
  Building2,
  User as UserIcon,
  LogOut,
  ShieldAlert,
  Check,
  Plus,
} from "lucide-react"
import { NotificationsPopover } from "@/components/notifications/notifications-popover"
import { NewItemDialog } from "@/components/items/new-item-dialog"

export function TopBar() {
  const {
    user,
    memberships,
    activeTeam,
    setActiveTeam,
    demoUsers,
    loginAs,
    logout,
    currentRole,
    isSystemAdmin,
  } = useAuth()

  const [newItemOpen, setNewItemOpen] = React.useState(false)

  const getInitials = (name?: string) => {
    if (!name) return "U"
    const parts = name.trim().split(" ")
    if (parts.length >= 2) {
      return `${parts[0][0]}${parts[parts.length - 1][0]}`.toUpperCase()
    }
    return name.slice(0, 2).toUpperCase()
  }

  const roleBadgeVariant = (role: string | null) => {
    switch (role) {
      case "lead":
        return "default"
      case "operator":
        return "secondary"
      default:
        return "outline"
    }
  }

  return (
    <header className="sticky top-0 z-30 flex h-14 w-full items-center justify-between border-b bg-background/95 px-4 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      {/* Team Filter / Selector */}
      <div className="flex items-center gap-2">
        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="outline" size="sm" className="gap-2 h-9 px-3 font-normal" />}>
            <Building2 className="size-4 text-muted-foreground" />
            <div className="flex items-center gap-1.5 text-left">
              <span className="font-semibold text-sm">
                {activeTeam ? activeTeam.team_name : "Select Team"}
              </span>
              {currentRole && (
                <Badge
                  variant={roleBadgeVariant(currentRole)}
                  className="text-[10px] uppercase font-semibold tracking-wider px-1.5 py-0"
                >
                  {currentRole}
                </Badge>
              )}
            </div>
            <ChevronDown className="size-3.5 opacity-50 ml-1" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-56">
            <DropdownMenuLabel>Your Teams</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              {memberships.length === 0 ? (
                <div className="px-2 py-1.5 text-xs text-muted-foreground">
                  No teams assigned
                </div>
              ) : (
                memberships.map((m) => {
                  const isSelected = activeTeam?.team_id === m.team_id
                  return (
                    <DropdownMenuItem
                      key={m.team_id}
                      onClick={() => setActiveTeam(m)}
                      className="flex items-center justify-between cursor-pointer"
                    >
                      <div className="flex flex-col">
                        <span className="font-medium text-sm">{m.team_name}</span>
                        <span className="text-xs text-muted-foreground capitalize">
                          {m.role}
                        </span>
                      </div>
                      {isSelected && <Check className="size-4 text-primary" />}
                    </DropdownMenuItem>
                  )
                })
              )}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Right side: New Item, Notifications, User Switcher dropdown */}
      <div className="flex items-center gap-2.5">
        <Button
          size="sm"
          onClick={() => setNewItemOpen(true)}
          className="h-8 gap-1.5 px-3 text-xs font-semibold shadow-xs"
        >
          <Plus className="size-3.5" />
          <span className="hidden sm:inline">New Item</span>
        </Button>

        <NotificationsPopover />

        {isSystemAdmin && (
          <Badge variant="destructive" className="gap-1 text-[11px] py-0.5 hidden md:flex">
            <ShieldAlert className="size-3" />
            System Admin
          </Badge>
        )}

        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="ghost" size="sm" className="gap-2 h-9 px-2 hover:bg-accent/80" />}>
            <Avatar className="size-7 border border-border">
              <AvatarFallback className="text-xs font-semibold bg-primary/10 text-primary">
                {getInitials(user?.name)}
              </AvatarFallback>
            </Avatar>
            <div className="hidden flex-col items-start text-left sm:flex">
              <span className="text-xs font-semibold leading-none">{user?.name || "Anonymous"}</span>
              <span className="text-[10px] text-muted-foreground leading-tight">{user?.email}</span>
            </div>
            <ChevronDown className="size-3.5 opacity-50" />
          </DropdownMenuTrigger>

          <DropdownMenuContent align="end" className="w-64">
            <DropdownMenuLabel>
              <div className="flex flex-col space-y-1">
                <p className="text-sm font-semibold">{user?.name}</p>
                <p className="text-xs text-muted-foreground truncate">{user?.email}</p>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />

            {/* Switch User Demo List */}
            <DropdownMenuLabel className="text-xs text-muted-foreground">
              Switch Demo User
            </DropdownMenuLabel>
            <DropdownMenuGroup>
              {demoUsers.map((u) => {
                const isCurrent = user?.id === u.id
                return (
                  <DropdownMenuItem
                    key={u.id}
                    onClick={() => loginAs(u.email)}
                    className="flex items-center justify-between cursor-pointer py-1.5"
                  >
                    <div className="flex items-center gap-2">
                      <UserIcon className="size-3.5 text-muted-foreground" />
                      <div className="flex flex-col">
                        <span className={`text-xs ${isCurrent ? "font-bold text-primary" : "font-medium"}`}>
                          {u.name}
                        </span>
                        <span className="text-[10px] text-muted-foreground">
                          {u.email}
                        </span>
                      </div>
                    </div>
                    {isCurrent && <Check className="size-3.5 text-primary" />}
                  </DropdownMenuItem>
                )
              })}
            </DropdownMenuGroup>

            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={logout}
              variant="destructive"
              className="cursor-pointer"
            >
              <LogOut className="size-4 mr-2" />
              <span>Sign Out</span>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <NewItemDialog open={newItemOpen} onOpenChange={setNewItemOpen} />
    </header>
  )
}
