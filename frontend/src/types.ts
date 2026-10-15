export type ApiErrorCode =
  | 'INVALID_INPUT'
  | 'ITEM_NOT_FOUND'
  | 'INSUFFICIENT_STOCK'
  | 'RESERVATION_NOT_FOUND'
  | 'RESERVATION_EXPIRED'
  | 'ALREADY_CONFIRMED'
  | 'INTERNAL_ERROR'

export interface StockResponse {
  item_id: string
  total_stock: number
  reserved_stock: number
  available_stock: number
}

export interface ReserveRequest {
  user_id: string
  item_id: string
  quantity: number
}

export interface ReserveResponse {
  status: 'success'
  reservation_id: string
  item_id: string
  quantity: number
  expires_at: string
}

export interface ConfirmRequest {
  reservation_id: string
}

export interface ConfirmResponse {
  status: 'success'
  reservation_id: string
  confirmed_at: string
}

export interface ApiErrorBody {
  code: ApiErrorCode
  message: string
  details?: Record<string, unknown>
}

export interface ActiveReservation {
  reservationId: string
  itemId: string
  quantity: number
  expiresAt: string
}
