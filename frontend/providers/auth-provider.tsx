"use client"

import * as React from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { apiFetch, getAuthToken, setAuthToken } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { User, Membership, MeResponse, DemoUsersResponse, DevLoginResponse } from "@/types"

interface AuthContextValue {
  user: User | null
  memberships: Membership[]
  activeTeam: Membership | null
  setActiveTeam: (team: Membership | null) => void
  isLoading: boolean
  isAuthenticated: boolean
  demoUsers: User[]
  isLoadingDemoUsers: boolean
  loginAs: (email: string) => Promise<void>
  logout: () => void
  isLead: boolean
  isOperator: boolean
  isSystemAdmin: boolean
  currentRole: string | null
}

const AuthContext = React.createContext<AuthContextValue | undefined>(undefined)

const ACTIVE_TEAM_KEY = "opsflow_active_team_id"

const FALLBACK_DEMO_USERS: User[] = [
  { id: "demo-alicia", email: "alicia@opsflow.local", name: "Alicia", is_system_admin: true },
  { id: "demo-marcus", email: "marcus@opsflow.local", name: "Marcus", is_system_admin: false },
  { id: "demo-priya", email: "priya@opsflow.local", name: "Priya", is_system_admin: false },
  { id: "demo-noah", email: "noah@opsflow.local", name: "Noah", is_system_admin: false },
  { id: "demo-elena", email: "elena@opsflow.local", name: "Elena", is_system_admin: false },
]

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient()
  const [token, setTokenState] = React.useState<string | null>(() => getAuthToken())
  const [activeTeamId, setActiveTeamIdState] = React.useState<string | null>(() => {
    if (typeof window !== "undefined") {
      try {
        return localStorage.getItem(ACTIVE_TEAM_KEY)
      } catch {
        return null
      }
    }
    return null
  })

  const { data: demoUsersData, isLoading: isLoadingDemoUsers } = useQuery({
    queryKey: queryKeys.auth.demoUsers(),
    queryFn: () => apiFetch<DemoUsersResponse>("/auth/demo-users", { skipAuth: true }),
    staleTime: 5 * 60 * 1000,
  })

  const { data: meData, isLoading: isLoadingMe } = useQuery({
    queryKey: queryKeys.auth.me(),
    queryFn: () => apiFetch<MeResponse>("/me"),
    enabled: !!token,
    staleTime: 2 * 60 * 1000,
  })

  const user = meData?.user ?? null
  const memberships = React.useMemo(() => meData?.memberships ?? [], [meData?.memberships])

  // Select active team
  const activeTeam = React.useMemo(() => {
    if (memberships.length === 0) {
      return null
    }
    if (activeTeamId) {
      const match = memberships.find((m) => m.team_id === activeTeamId)
      if (match) return match
    }
    return memberships[0]
  }, [memberships, activeTeamId])

  const setActiveTeam = React.useCallback((team: Membership | null) => {
    const id = team?.team_id ?? null
    setActiveTeamIdState(id)
    if (typeof window !== "undefined") {
      try {
        if (id) {
          localStorage.setItem(ACTIVE_TEAM_KEY, id)
        } else {
          localStorage.removeItem(ACTIVE_TEAM_KEY)
        }
      } catch {
        // storage disabled
      }
    }
  }, [])

  const loginAs = React.useCallback(
    async (email: string) => {
      const res = await apiFetch<DevLoginResponse>("/auth/dev-login", {
        method: "POST",
        body: { email },
        skipAuth: true,
      })
      setAuthToken(res.access_token)
      setTokenState(res.access_token)
      await queryClient.invalidateQueries({ queryKey: queryKeys.auth.me() })
    },
    [queryClient]
  )

  const logout = React.useCallback(() => {
    setAuthToken(null)
    setTokenState(null)
    setActiveTeam(null)
    queryClient.removeQueries()
  }, [queryClient, setActiveTeam])

  const isSystemAdmin = !!user?.is_system_admin
  const isLead = isSystemAdmin || activeTeam?.role === "lead"
  const isOperator = isLead || activeTeam?.role === "operator"
  const currentRole = activeTeam?.role ?? null

  const value = React.useMemo<AuthContextValue>(
    () => ({
      user,
      memberships,
      activeTeam,
      setActiveTeam,
      isLoading: !!token && isLoadingMe,
      isAuthenticated: !!user,
      demoUsers:
        demoUsersData?.users && demoUsersData.users.length > 0
          ? demoUsersData.users
          : FALLBACK_DEMO_USERS,
      isLoadingDemoUsers,
      loginAs,
      logout,
      isLead,
      isOperator,
      isSystemAdmin,
      currentRole,
    }),
    [
      user,
      memberships,
      activeTeam,
      setActiveTeam,
      token,
      isLoadingMe,
      demoUsersData?.users,
      isLoadingDemoUsers,
      loginAs,
      logout,
      isLead,
      isOperator,
      isSystemAdmin,
      currentRole,
    ]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const context = React.useContext(AuthContext)
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider")
  }
  return context
}
