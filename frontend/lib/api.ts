export class ApiError extends Error {
  readonly code: string
  readonly status: number
  readonly details: Record<string, unknown>
  readonly requestId: string

  constructor(
    code: string,
    status: number,
    message: string,
    details: Record<string, unknown> = {},
    requestId = ""
  ) {
    super(message)
    this.name = "ApiError"
    this.code = code
    this.status = status
    this.details = details
    this.requestId = requestId
  }
}

const TOKEN_STORAGE_KEY = "opsflow_token"
let inMemoryToken: string | null = null

export function getAuthToken(): string | null {
  if (inMemoryToken) {
    return inMemoryToken
  }
  if (typeof window !== "undefined") {
    try {
      const stored = localStorage.getItem(TOKEN_STORAGE_KEY)
      if (stored) {
        inMemoryToken = stored
        return stored
      }
    } catch {
      // localStorage may be unavailable or restricted
    }
  }
  return null
}

export function setAuthToken(token: string | null): void {
  inMemoryToken = token
  if (typeof window !== "undefined") {
    try {
      if (token) {
        localStorage.setItem(TOKEN_STORAGE_KEY, token)
      } else {
        localStorage.removeItem(TOKEN_STORAGE_KEY)
      }
    } catch {
      // localStorage may be unavailable or restricted
    }
  }
}

export interface RequestOptions extends Omit<RequestInit, "body"> {
  body?: unknown
  ifMatch?: number | string
  idempotencyKey?: string
  skipAuth?: boolean
}

export async function apiFetch<T>(
  path: string,
  options: RequestOptions = {}
): Promise<T> {
  const baseUrl = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"
  const url = path.startsWith("http") ? path : `${baseUrl}${path.startsWith("/") ? "" : "/"}${path}`

  const headers = new Headers(options.headers || {})

  if (!options.skipAuth) {
    const token = getAuthToken()
    if (token && !headers.has("Authorization")) {
      headers.set("Authorization", `Bearer ${token}`)
    }
  }

  if (options.ifMatch !== undefined) {
    headers.set("If-Match", String(options.ifMatch))
  }

  if (options.idempotencyKey) {
    headers.set("Idempotency-Key", options.idempotencyKey)
  }

  let body: BodyInit | undefined
  if (options.body !== undefined) {
    if (
      typeof options.body === "string" ||
      options.body instanceof FormData ||
      options.body instanceof Blob ||
      options.body instanceof ArrayBuffer
    ) {
      body = options.body as BodyInit
    } else {
      if (!headers.has("Content-Type")) {
        headers.set("Content-Type", "application/json")
      }
      body = JSON.stringify(options.body)
    }
  }

  const response = await fetch(url, {
    ...options,
    headers,
    body,
  })

  const requestIdHeader = response.headers.get("X-Request-Id") || ""

  if (!response.ok) {
    let errorCode = "unknown_error"
    let errorMessage = `Request failed with status ${response.status}`
    let errorDetails: Record<string, unknown> = {}
    let requestId = requestIdHeader

    try {
      const data = await response.json()
      if (data && typeof data === "object" && "error" in data) {
        const errObj = (data as { error: { code?: string; message?: string; request_id?: string; details?: Record<string, unknown> } }).error
        if (errObj) {
          errorCode = errObj.code || errorCode
          errorMessage = errObj.message || errorMessage
          errorDetails = errObj.details || {}
          requestId = errObj.request_id || requestId
        }
      } else if (data && typeof data === "object" && "message" in data) {
        errorMessage = String((data as { message: unknown }).message)
      }
    } catch {
      // response body was not valid JSON
    }

    throw new ApiError(
      errorCode,
      response.status,
      errorMessage,
      errorDetails,
      requestId
    )
  }

  if (response.status === 204) {
    return undefined as unknown as T
  }

  const contentType = response.headers.get("content-type") || ""
  if (contentType.includes("application/json")) {
    return (await response.json()) as T
  }

  return (await response.text()) as unknown as T
}
