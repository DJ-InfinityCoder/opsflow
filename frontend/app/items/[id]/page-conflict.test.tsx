import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { ApiError } from "@/lib/api"
import ItemDetailPage from "./page"

const item = {
  id: "item-1",
  team_id: "team-1",
  title: "Original title",
  description: "Original description",
  status: "new",
  priority: 3,
  assignee_id: null,
  assignee_name: null,
  created_by: "creator-1",
  custom_fields: {},
  version: 1,
  created_at: "2025-01-01T00:00:00Z",
  updated_at: "2025-01-01T00:05:00Z",
  allowed_transitions: ["triaged"],
}

const serverItem = {
  ...item,
  title: "Server title",
  description: "Server description",
  priority: 2,
  version: 2,
  updated_at: "2025-01-01T00:10:00Z",
}

vi.mock("next/navigation", () => ({
  useParams: () => ({ id: "item-1" }),
  useRouter: () => ({ replace: vi.fn(), push: vi.fn(), prefetch: vi.fn() }),
  usePathname: () => "/items/item-1",
  useSearchParams: () => new URLSearchParams(),
}))

vi.mock("@/providers/auth-provider", () => ({
  useAuth: () => ({
    user: { id: "user-1", email: "alice@opsflow.local", name: "Alice", is_system_admin: false },
    isLead: false,
    memberships: [],
  }),
}))

const apiFetchMock = vi.fn()

vi.mock("@/lib/api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api")>("@/lib/api")
  return {
    ...actual,
    apiFetch: (...args: Parameters<typeof actual.apiFetch>) => apiFetchMock(...args),
  }
})

describe("ItemDetailPage conflict dialog", () => {
  beforeEach(() => {
    apiFetchMock.mockReset()
    apiFetchMock.mockImplementation(async (path: string, options: { method?: string; ifMatch?: number; body?: Record<string, unknown> } = {}) => {
      if (path === "/items/item-1" && options.method === "PATCH") {
        if (options.ifMatch === 1) {
          throw new ApiError("version_conflict", 409, "version conflict", {
            current_item: serverItem,
          })
        }

        return {
          ...serverItem,
          ...options.body,
          version: 2,
          updated_at: new Date().toISOString(),
        }
      }

      if (path === "/items/item-1") {
        return item
      }

      if (path === "/teams/team-1/field-schemas") {
        return []
      }

      if (path === "/teams/team-1/members") {
        return []
      }

      if (path === "/items/item-1/events") {
        return { events: [] }
      }

      if (path === "/items/item-1/comments") {
        return { comments: [] }
      }

      throw new Error(`Unhandled apiFetch path: ${path}`)
    })
  })

  it("shows the conflict dialog and retries with the server version", async () => {
    const user = userEvent.setup()
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    render(
      <QueryClientProvider client={queryClient}>
        <ItemDetailPage />
      </QueryClientProvider>
    )

    await waitFor(() => {
      expect(screen.getByDisplayValue("Original title")).toBeInTheDocument()
    })

    const titleInput = screen.getByLabelText("Title")
    await user.clear(titleInput)
    await user.type(titleInput, "My updated title")

    await user.click(screen.getByRole("button", { name: /save changes/i }))

    await waitFor(() => {
      expect(screen.getByText("Version Conflict (409)")).toBeInTheDocument()
    })

    const chooseTheirsButtons = screen.getAllByRole("button", { name: /use theirs/i })
    await user.click(chooseTheirsButtons[0])

    await user.click(screen.getByRole("button", { name: /apply & retry save/i }))

    await waitFor(() => {
      const retryCall = apiFetchMock.mock.calls.find(
        ([path, options]) => path === "/items/item-1" && options?.method === "PATCH" && options?.ifMatch === 2
      )

      expect(retryCall).toBeTruthy()
    })
  }, 15000)
})
