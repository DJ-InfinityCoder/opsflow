"use client"

import * as React from "react"
import { useRouter, usePathname } from "next/navigation"
import { useAuth } from "@/providers/auth-provider"
import { Sidebar } from "@/components/layout/sidebar"
import { TopBar } from "@/components/layout/top-bar"
import { Skeleton } from "@/components/ui/skeleton"

import { BackendStatusBanner } from "@/components/layout/backend-status-banner"

export function AppShell({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const pathname = usePathname()
  const { isAuthenticated, isLoading } = useAuth()
  const [mounted, setMounted] = React.useState(false)

  React.useEffect(() => {
    setMounted(true)
  }, [])

  const isLoginPage = pathname === "/login"

  React.useEffect(() => {
    if (mounted && !isLoading && !isAuthenticated && !isLoginPage) {
      router.push("/login")
    }
  }, [mounted, isAuthenticated, isLoading, isLoginPage, router])

  if (isLoginPage) {
    return (
      <div className="flex min-h-screen flex-col">
        <BackendStatusBanner />
        <div className="flex-1 flex items-center justify-center">
          {children}
        </div>
      </div>
    )
  }

  if (!mounted || isLoading) {
    return (
      <div className="flex h-screen w-full items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-4">
          <div className="size-10 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <div className="space-y-2 text-center">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-3 w-24" />
          </div>
        </div>
      </div>
    )
  }

  if (!isAuthenticated) {
    return null
  }

  return (
    <div className="flex h-screen w-full overflow-hidden bg-background">
      <Sidebar />
      <div className="flex flex-1 flex-col overflow-hidden">
        <TopBar />
        <BackendStatusBanner />
        <main className="flex-1 overflow-y-auto p-4 md:p-6 lg:p-8 pb-20 md:pb-28">
          {children}
        </main>
      </div>
    </div>
  )
}
