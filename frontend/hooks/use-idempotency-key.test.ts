import { act, renderHook } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { useIdempotencyKey } from "./use-idempotency-key"

describe("useIdempotencyKey", () => {
  it("reuses the same key for the same intent and rotates it for a new intent", () => {
    const { result } = renderHook(() => useIdempotencyKey())

    const firstKey = result.current.read()
    expect(result.current.read()).toBe(firstKey)

    act(() => {
      result.current.resetForNewIntent()
    })

    expect(result.current.read()).not.toBe(firstKey)
  })
})
