import * as React from "react"

export function useIdempotencyKey() {
  const keyRef = React.useRef<string>(crypto.randomUUID())

  const read = React.useCallback(() => keyRef.current, [])
  const resetForNewIntent = React.useCallback(() => {
    keyRef.current = crypto.randomUUID()
  }, [])

  return { read, resetForNewIntent }
}
