import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { renderHook, waitFor } from "@testing-library/react"
import { act } from "react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import * as apiModule from "@/lib/api"
import { ApiError } from "@/lib/api"
import type { User, WorkItem } from "@/types"
import { toast } from "sonner"

import { useClaimWorkItem } from "./use-claim-work-item"

const currentUser: User = {
  id: "user-123",
  email: "me@opsflow.local",
  name: "Me",
  is_system_admin: false,
}

function makeItem(overrides: Partial<WorkItem> = {}): WorkItem {
  return {
    id: "item-1",
    team_id: "team-1",
    title: "Payment investigation",
    description: "Investigate a customer payment issue",
    status: "new",
    priority: 2,
    assignee_id: null,
    assignee_name: null,
    created_by: "creator-1",
    custom_fields: {},
    version: 7,
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-01T00:00:00Z",
    ...overrides,
  }
}

describe("useClaimWorkItem", () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it("rolls back the optimistic cache when another operator already claimed it", async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    const activeQueryKey = ["items", "list", { view: "all" }] as const
    const originalItem = makeItem()
    const cachedData = {
      pages: [{ items: [originalItem] }],
      pageParams: [undefined],
    }
    queryClient.setQueryData(activeQueryKey, cachedData)

    const winner = { assignee_id: "winner-42", assignee_name: "Alicia" }
    const serverItem = makeItem({
      assignee_id: winner.assignee_id,
      assignee_name: winner.assignee_name,
      status: "in_progress",
      version: 8,
    })

    const apiFetchSpy = vi.spyOn(apiModule, "apiFetch").mockRejectedValue(
      new ApiError("already_claimed", 409, "already claimed", {
        winner,
        current_item: serverItem,
      })
    )

    const toastErrorSpy = vi.spyOn(toast, "error").mockImplementation(() => "toast-1")

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useClaimWorkItem(activeQueryKey, currentUser), { wrapper })

    await act(async () => {
      result.current.mutate(originalItem)
    })

    await waitFor(() => {
      expect(queryClient.getQueryData(activeQueryKey)).toEqual({
        pages: [{ items: [serverItem] }],
        pageParams: [undefined],
      })
      expect(toastErrorSpy).toHaveBeenCalledWith("winner-42 claimed this just now")
    })

    expect(apiFetchSpy).toHaveBeenCalledTimes(1)
    expect(queryClient.getQueryData(activeQueryKey)).toEqual({
      pages: [{ items: [serverItem] }],
      pageParams: [undefined],
    })
  })
})
