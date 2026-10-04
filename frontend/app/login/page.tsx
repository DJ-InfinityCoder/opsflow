"use client"

import * as React from "react"
import { useRouter } from "next/navigation"
import { useAuth } from "@/providers/auth-provider"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { toast } from "sonner"
import {
  Layers,
  ArrowRight,
  ShieldCheck,
  User as UserIcon,
  Sparkles,
  Mail,
  Loader2,
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
    <div className="flex min-h-screen w-full items-center justify-center bg-background p-4 sm:p-6 lg:p-8">
      <Card className="w-full max-w-md border border-border bg-card shadow-none">
        <CardHeader className="text-center pb-4 pt-6">
          <div className="mx-auto mb-3 flex size-12 items-center justify-center rounded-sm bg-primary text-primary-foreground shadow-none">
            <Layers className="size-6" />
          </div>
          <CardTitle className="text-xl font-bold tracking-tight">
            OpsFlow Operations
          </CardTitle>
          <CardDescription className="text-xs text-muted-foreground mt-1">
            Work-item tracking & incident orchestration system
          </CardDescription>
        </CardHeader>

        <CardContent className="space-y-6 pt-2 pb-6">
          {/* Quick Click Demo Personas */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
                <Sparkles className="size-3.5 text-primary" />
                <span>Quick Sign-In Personas</span>
              </Label>
              {isLoadingDemoUsers && (
                <span className="text-[10px] text-muted-foreground flex items-center gap-1">
                  <Loader2 className="size-3 animate-spin" /> Loading...
                </span>
              )}
            </div>

            <div className="grid grid-cols-1 gap-2">
              {demoUsers.slice(0, 4).map((u) => (
                <button
                  key={u.id}
                  type="button"
                  onClick={() => handleLogin(u.email)}
                  disabled={isSubmitting}
                  className="flex w-full items-center justify-between rounded-sm border border-border bg-muted/20 p-2.5 text-left text-xs transition-colors hover:bg-muted/60 hover:border-foreground/30 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                >
                  <div className="flex items-center gap-2.5">
                    <div className="flex size-7 items-center justify-center rounded-sm bg-secondary text-foreground">
                      <UserIcon className="size-3.5" />
                    </div>
                    <div>
                      <div className="font-semibold text-foreground leading-tight">
                        {u.name}
                      </div>
                      <div className="text-[11px] text-muted-foreground">
                        {u.email}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    {u.is_system_admin ? (
                      <Badge variant="destructive" className="text-[10px] px-1.5 py-0 h-4">
                        <ShieldCheck className="size-3 mr-0.5 inline" /> Admin
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-[10px] px-1.5 py-0 h-4">
                        Operator
                      </Badge>
                    )}
                    <ArrowRight className="size-3.5 text-muted-foreground" />
                  </div>
                </button>
              ))}
            </div>
          </div>

          {/* Divider */}
          <div className="relative flex items-center justify-center">
            <div className="border-t border-border w-full" />
            <span className="absolute bg-card px-2 text-[11px] uppercase tracking-wider text-muted-foreground">
              Or enter email
            </span>
          </div>

          {/* Manual Email Login Form */}
          <form
            onSubmit={(e) => {
              e.preventDefault()
              handleLogin(emailInput)
            }}
            className="space-y-3"
          >
            <div className="space-y-1.5">
              <Label htmlFor="email" className="text-xs font-medium">
                Email Address
              </Label>
              <div className="relative">
                <Mail className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground" />
                <Input
                  id="email"
                  type="email"
                  placeholder="alicia@opsflow.local"
                  value={emailInput}
                  onChange={(e) => setEmailInput(e.target.value)}
                  disabled={isSubmitting}
                  className="pl-9 h-10 text-xs shadow-none"
                />
              </div>
            </div>

            <Button
              type="submit"
              className="w-full h-10 gap-2 font-medium text-xs shadow-none"
              disabled={isSubmitting || !emailInput.trim()}
            >
              {isSubmitting ? (
                <>
                  <Loader2 className="size-3.5 animate-spin" />
                  <span>Signing In...</span>
                </>
              ) : (
                <>
                  <span>Sign In to OpsFlow</span>
                  <ArrowRight className="size-3.5" />
                </>
              )}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
