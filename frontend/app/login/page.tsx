"use client"

import * as React from "react"
import { useRouter } from "next/navigation"
import { useAuth } from "@/providers/auth-provider"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { toast } from "sonner"
import {
  Layers,
  ChevronDown,
  ArrowRight,
  ShieldAlert,
  UserCheck,
  User as UserIcon,
  Sparkles,
} from "lucide-react"

export default function LoginPage() {
  const router = useRouter()
  const { demoUsers, isLoadingDemoUsers, loginAs, isAuthenticated } = useAuth()
  const [emailInput, setEmailInput] = React.useState("")
  const [isSubmitting, setIsSubmitting] = React.useState(false)

  React.useEffect(() => {
    if (isAuthenticated) {
      router.push("/")
    }
  }, [isAuthenticated, router])

  const handleLogin = async (email: string) => {
    if (!email.trim()) {
      toast.error("Please provide a valid email address.")
      return
    }

    try {
      setIsSubmitting(true)
      await loginAs(email)
      toast.success(`Signed in as ${email}`)
      router.push("/")
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to sign in"
      toast.error(msg)
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="relative flex min-h-screen w-full items-center justify-center overflow-hidden bg-gradient-to-br from-background via-muted/30 to-background p-4 sm:p-6 lg:p-8">
      {/* Background Decorative Elements */}
      <div className="absolute top-1/4 left-1/4 -z-10 size-96 rounded-full bg-primary/10 blur-3xl pointer-events-none" />
      <div className="absolute bottom-1/4 right-1/4 -z-10 size-96 rounded-full bg-secondary/15 blur-3xl pointer-events-none" />

      <Card className="w-full max-w-md border-border/80 bg-card/90 shadow-2xl backdrop-blur-xl">
        <CardHeader className="text-center pb-4">
          <div className="mx-auto mb-3 flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-md">
            <Layers className="size-6" />
          </div>
          <CardTitle className="text-2xl font-bold tracking-tight">
            OpsFlow Operations
          </CardTitle>
          <CardDescription className="text-xs text-muted-foreground">
            Work-item tracking & incident orchestration system
          </CardDescription>
        </CardHeader>

        <CardContent className="space-y-6 pt-2">
          {/* Quick Demo User Dropdown */}
          <div className="space-y-2">
            <Label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
              Quick Switch Demo Persona
            </Label>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant="outline"
                    className="w-full justify-between h-11 border-dashed font-normal text-sm"
                    disabled={isSubmitting || isLoadingDemoUsers}
                  />
                }
              >
                <div className="flex items-center gap-2">
                  <Sparkles className="size-4 text-primary" />
                  <span>Choose a Demo User...</span>
                </div>
                <ChevronDown className="size-4 opacity-50" />
              </DropdownMenuTrigger>

              <DropdownMenuContent align="center" className="w-(--anchor-width) min-w-80">
                <DropdownMenuLabel>Available Seeded Personas</DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuGroup>
                  {demoUsers.length === 0 ? (
                    <div className="p-3 text-center text-xs text-muted-foreground">
                      {isLoadingDemoUsers ? "Loading demo users..." : "No demo users found"}
                    </div>
                  ) : (
                    demoUsers.map((u) => (
                      <DropdownMenuItem
                        key={u.id}
                        onClick={() => handleLogin(u.email)}
                        className="flex items-center justify-between p-2.5 cursor-pointer"
                      >
                        <div className="flex items-center gap-2.5">
                          <UserCheck className="size-4 text-primary" />
                          <div className="flex flex-col text-left">
                            <span className="text-xs font-semibold">{u.name}</span>
                            <span className="text-[10px] text-muted-foreground">{u.email}</span>
                          </div>
                        </div>
                        {u.is_system_admin ? (
                          <Badge variant="destructive" className="text-[9px] px-1 py-0">
                            Admin
                          </Badge>
                        ) : (
                          <Badge variant="secondary" className="text-[9px] px-1 py-0">
                            Member
                          </Badge>
                        )}
                      </DropdownMenuItem>
                    ))
                  )}
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>

          {/* Quick Click Demo Cards */}
          {demoUsers.length > 0 && (
            <div className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                Or Select Directly
              </Label>
              <div className="grid grid-cols-1 gap-2">
                {demoUsers.slice(0, 4).map((u) => (
                  <button
                    key={u.id}
                    type="button"
                    onClick={() => handleLogin(u.email)}
                    disabled={isSubmitting}
                    className="flex w-full items-center justify-between rounded-lg border border-border/70 bg-background/50 p-2.5 text-left text-xs transition-all hover:bg-accent/60 hover:border-primary/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <div className="flex items-center gap-2.5">
                      <div className="flex size-7 items-center justify-center rounded-full bg-primary/10 text-primary">
                        <UserIcon className="size-3.5" />
                      </div>
                      <div>
                        <div className="font-semibold text-foreground">{u.name}</div>
                        <div className="text-[10px] text-muted-foreground">{u.email}</div>
                      </div>
                    </div>
                    {u.is_system_admin ? (
                      <span className="flex items-center gap-1 text-[10px] font-medium text-destructive">
                        <ShieldAlert className="size-3" />
                        Admin
                      </span>
                    ) : (
                      <ArrowRight className="size-3.5 text-muted-foreground" />
                    )}
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Manual Email Login Form */}
          <form
            onSubmit={(e) => {
              e.preventDefault()
              handleLogin(emailInput)
            }}
            className="space-y-3 pt-2 border-t"
          >
            <div className="space-y-1.5">
              <Label htmlFor="email" className="text-xs font-medium">
                Sign in with custom email
              </Label>
              <Input
                id="email"
                type="email"
                placeholder="operator@example.com"
                value={emailInput}
                onChange={(e) => setEmailInput(e.target.value)}
                disabled={isSubmitting}
                className="h-10 text-xs"
              />
            </div>

            <Button
              type="submit"
              className="w-full h-10 gap-2 font-semibold"
              disabled={isSubmitting || !emailInput.trim()}
            >
              <span>{isSubmitting ? "Authenticating..." : "Sign In to OpsFlow"}</span>
              <ArrowRight className="size-4" />
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
