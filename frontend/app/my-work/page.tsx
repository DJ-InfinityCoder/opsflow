"use client"

import * as React from "react"
import { useAuth } from "@/providers/auth-provider"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { CheckCircle2 } from "lucide-react"

export default function MyWorkPage() {
  const { user, activeTeam } = useAuth()

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-1">
        <h1 className="text-2xl font-bold tracking-tight text-foreground sm:text-3xl">
          My Work
        </h1>
        <p className="text-sm text-muted-foreground">
          Work items assigned to {user?.name || "you"} in {activeTeam?.team_name || "your active team"}.
        </p>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-base font-semibold">Assigned Items Queue</CardTitle>
            <Badge variant="secondary">0 items</Badge>
          </div>
          <CardDescription className="text-xs">
            Items currently claimed or assigned for investigation.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col items-center justify-center p-8 text-center text-muted-foreground">
            <CheckCircle2 className="size-10 text-muted-foreground/40 mb-3" />
            <p className="text-sm font-medium">All caught up!</p>
            <p className="text-xs text-muted-foreground mt-1 max-w-sm">
              You do not have any items currently assigned in {activeTeam?.team_name || "this team"}.
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
