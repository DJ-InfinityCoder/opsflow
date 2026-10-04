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
import { ThemeToggle } from "@/components/layout/theme-toggle"

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
    if (!name || !name.trim()) return "U"
    return name.trim().charAt(0).toUpperCase()
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
          <DropdownMenuTrigger render={<Button variant="outline" size="sm" className="gap-2 h-8 px-2.5 rounded-sm border border-border bg-background hover:bg-accent font-normal text-xs" />}>
            <Building2 className="size-3.5 text-muted-foreground" />
            <div className="flex items-center gap-1.5 text-left">
              <span className="font-semibold text-xs text-foreground">
                {activeTeam ? activeTeam.team_name : "Select Team"}
              </span>
              {currentRole && (
                <Badge
                  variant={roleBadgeVariant(currentRole)}
                  className="text-[9px] uppercase font-semibold tracking-wider px-1.5 py-0 h-4"
                >
                  {currentRole}
                </Badge>
              )}
            </div>
            <ChevronDown className="size-3 opacity-50 ml-0.5" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-56 border border-border bg-popover shadow-none rounded-md">
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
                        <span className="font-medium text-xs">{m.team_name}</span>
                        <span className="text-[10px] text-muted-foreground capitalize">
                          {m.role}
                        </span>
                      </div>
                      {isSelected && <Check className="size-3.5 text-primary" />}
                    </DropdownMenuItem>
                  )
                })
              )}
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Right side: New Item, Notifications, User Switcher dropdown */}
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          onClick={() => setNewItemOpen(true)}
          className="h-8 gap-1.5 px-3 text-xs font-semibold rounded-sm border border-primary bg-primary text-primary-foreground hover:bg-primary/90 shadow-none"
        >
          <Plus className="size-3.5" />
          <span className="hidden sm:inline">New Item</span>
        </Button>

        <NotificationsPopover />

        <ThemeToggle />

        {isSystemAdmin && (
          <Badge variant="destructive" className="gap-1 text-[10px] py-0.5 h-6 hidden md:flex border border-destructive/30">
            <ShieldAlert className="size-3" />
            System Admin
          </Badge>
        )}

        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="outline" size="sm" className="gap-2 h-8 px-2 rounded-sm border border-border bg-background hover:bg-accent text-foreground font-normal" />}>
            <Avatar className="size-5 border border-border">
              <AvatarFallback className="text-[10px] font-bold bg-primary text-primary-foreground">
                {getInitials(user?.name)}
              </AvatarFallback>
            </Avatar>
            <div className="hidden flex-col items-start text-left sm:flex">
              <span className="text-xs font-medium leading-none">{user?.name || "User"}</span>
            </div>
            <ChevronDown className="size-3 opacity-50" />
          </DropdownMenuTrigger>

          <DropdownMenuContent align="end" className="w-64 border border-border bg-popover shadow-none rounded-md">
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
