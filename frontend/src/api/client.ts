import type {
  ApiErrorBody,
  ApiErrorCode,
  ConfirmRequest,
  ConfirmResponse,
  ReserveRequest,
  ReserveResponse,
  StockResponse,
} from '../types'

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? ''

export class ApiError extends Error {
  readonly code: ApiErrorCode
  readonly status: number
  readonly details?: Record<string, unknown>

  constructor(
    code: ApiErrorCode,
    message: string,
    status: number,
    details?: Record<string, unknown>,
  ) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.details = details
  }
}

export function toApiError(err: unknown): ApiError {
  return err instanceof ApiError ? err : new ApiError('INTERNAL_ERROR', 'Unexpected error.', 0)
}

interface ErrorEnvelope {
  error?: Partial<ApiErrorBody>
}

function parseJson(text: string): unknown {
  if (!text) {
    return null
  }
  try {
    return JSON.parse(text)
  } catch {
    return null
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response
  try {
    response = await fetch(`${API_BASE}${path}`, {
      ...init,
      headers: {
        Accept: 'application/json',
        ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
        ...init?.headers,
      },
    })
  } catch {
    throw new ApiError('INTERNAL_ERROR', 'Unable to reach the server.', 0)
  }

  const payload = parseJson(await response.text())

  if (!response.ok) {
    const envelope = payload as ErrorEnvelope | null
    throw new ApiError(
      envelope?.error?.code ?? 'INTERNAL_ERROR',
      envelope?.error?.message ?? `Request failed with status ${response.status}.`,
      response.status,
      envelope?.error?.details,
    )
  }

  return payload as T
}

export function getStock(itemId: string): Promise<StockResponse> {
  return request<StockResponse>(`/api/v1/inventory/stock?item_id=${encodeURIComponent(itemId)}`)
}

export function reserveStock(body: ReserveRequest): Promise<ReserveResponse> {
  return request<ReserveResponse>('/api/v1/inventory/reserve', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export function confirmReservation(body: ConfirmRequest): Promise<ConfirmResponse> {
  return request<ConfirmResponse>('/api/v1/inventory/confirm', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}
