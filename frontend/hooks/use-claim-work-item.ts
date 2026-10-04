import { useMutation, useQueryClient, type InfiniteData, type QueryKey } from "@tanstack/react-query"
import { ApiError, apiFetch } from "@/lib/api"
import { queryKeys } from "@/lib/query-keys"
import type { ItemListResult, User, WorkItem } from "@/types"
import { toast } from "sonner"

export function useClaimWorkItem(activeQueryKey: QueryKey, user: User | null) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (item: WorkItem) => {
      return apiFetch<WorkItem>(`/items/${item.id}/claim`, {
        method: "POST",
        body: {},
        idempotencyKey: crypto.randomUUID(),
      })
    },
    onMutate: async (item) => {
      await queryClient.cancelQueries({ queryKey: activeQueryKey })
      await queryClient.cancelQueries({ queryKey: queryKeys.views.counts() })

      const previousData = queryClient.getQueryData<InfiniteData<ItemListResult>>(activeQueryKey)
      queryClient.setQueryData<InfiniteData<ItemListResult>>(activeQueryKey, (old) => {
        if (!old) return old
        return {
          ...old,
          pages: old.pages.map((page) => ({
            ...page,
            items: page.items.map((row) =>
              row.id === item.id
                ? {
                    ...row,
                    assignee_id: user?.id ?? "me",
                    assignee_name: user?.name ?? "Me",
                    status: "in_progress",
                    version: row.version + 1,
                  }
                : row
            ),
          })),
        }
      })

      return { previousData }
    },
    onError: (error, _item, context) => {
      if (context?.previousData) {
        queryClient.setQueryData(activeQueryKey, context.previousData)
      }

      if (error instanceof ApiError && (error.status === 409 || error.code === "already_claimed")) {
        const winner = error.details?.winner as { assignee_id?: string } | undefined
        const winnerId = winner?.assignee_id || "Another operator"
        toast.error(`${winnerId} claimed this just now`)

        const serverItem = error.details?.current_item as WorkItem | undefined
        if (serverItem) {
          queryClient.setQueryData<InfiniteData<ItemListResult>>(activeQueryKey, (old) => {
            if (!old) return old
            return {
              ...old,
              pages: old.pages.map((page) => ({
                ...page,
                items: page.items.map((row) => (row.id === serverItem.id ? serverItem : row)),
              })),
            }
          })
        } else {
          queryClient.invalidateQueries({ queryKey: activeQueryKey })
        }
      } else {
        toast.error(error instanceof Error ? error.message : "Failed to claim item")
      }
    },
    onSuccess: (updatedItem) => {
      toast.success("Item claimed successfully")
      queryClient.setQueryData<InfiniteData<ItemListResult>>(activeQueryKey, (old) => {
        if (!old) return old
        return {
          ...old,
          pages: old.pages.map((page) => ({
            ...page,
            items: page.items.map((row) => (row.id === updatedItem.id ? updatedItem : row)),
          })),
        }
      })
      queryClient.invalidateQueries({ queryKey: queryKeys.views.counts() })
    },
  })
}
