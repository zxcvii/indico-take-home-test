import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, confirmReservation, getStock, reserveStock, toApiError } from './client'
import { errorResponse, jsonResponse } from '../test/helpers'

function stubFetch(response: Response) {
  const fetchMock = vi.fn().mockResolvedValue(response)
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('getStock', () => {
  it('calls the stock endpoint with an encoded item id', async () => {
    const fetchMock = stubFetch(
      jsonResponse({ item_id: 'item 4021', total_stock: 100, reserved_stock: 15, available_stock: 85 }),
    )

    const stock = await getStock('item 4021')

    expect(stock).toEqual({
      item_id: 'item 4021',
      total_stock: 100,
      reserved_stock: 15,
      available_stock: 85,
    })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/inventory/stock?item_id=item%204021')
    expect(init.headers).toMatchObject({ Accept: 'application/json' })
  })
})

describe('reserveStock', () => {
  it('posts the reservation body as JSON', async () => {
    const fetchMock = stubFetch(
      jsonResponse(
        {
          status: 'success',
          reservation_id: 'res_1',
          item_id: 'item_4021',
          quantity: 2,
          expires_at: '2026-01-01T00:05:00Z',
        },
        201,
      ),
    )

    const reservation = await reserveStock({ user_id: 'usr_1', item_id: 'item_4021', quantity: 2 })

    expect(reservation.reservation_id).toBe('res_1')
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/inventory/reserve')
    expect(init).toMatchObject({ method: 'POST' })
    expect(init.headers).toMatchObject({ 'Content-Type': 'application/json' })
    expect(JSON.parse(String(init.body))).toEqual({ user_id: 'usr_1', item_id: 'item_4021', quantity: 2 })
  })
})

describe('confirmReservation', () => {
  it('posts the reservation id', async () => {
    const fetchMock = stubFetch(
      jsonResponse({ status: 'success', reservation_id: 'res_1', confirmed_at: '2026-01-01T00:02:00Z' }),
    )

    const result = await confirmReservation({ reservation_id: 'res_1' })

    expect(result.confirmed_at).toBe('2026-01-01T00:02:00Z')
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/inventory/confirm')
    expect(JSON.parse(String(init.body))).toEqual({ reservation_id: 'res_1' })
  })
})

describe('error handling', () => {
  it('throws a typed ApiError carrying the envelope code, status and details', async () => {
    stubFetch(
      errorResponse('INSUFFICIENT_STOCK', 'Only 3 units available, requested 5.', 409, {
        item_id: 'item_4021',
        available: 3,
        requested: 5,
      }),
    )

    const promise = reserveStock({ user_id: 'usr_1', item_id: 'item_4021', quantity: 5 })

    await expect(promise).rejects.toBeInstanceOf(ApiError)
    await expect(promise).rejects.toMatchObject({
      code: 'INSUFFICIENT_STOCK',
      status: 409,
      message: 'Only 3 units available, requested 5.',
      details: { item_id: 'item_4021', available: 3, requested: 5 },
    })
  })

  it('falls back to INTERNAL_ERROR for a non-JSON error body', async () => {
    stubFetch({ ok: false, status: 500, text: async () => '<html>oops</html>' } as unknown as Response)

    await expect(getStock('item_4021')).rejects.toMatchObject({ code: 'INTERNAL_ERROR', status: 500 })
  })

  it('maps a network failure to an INTERNAL_ERROR ApiError', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('boom')))

    await expect(getStock('item_4021')).rejects.toMatchObject({ code: 'INTERNAL_ERROR', status: 0 })
  })
})

describe('toApiError', () => {
  it('returns the same ApiError instance unchanged', () => {
    const original = new ApiError('ITEM_NOT_FOUND', 'nope', 404)

    expect(toApiError(original)).toBe(original)
  })

  it('wraps unknown errors as INTERNAL_ERROR', () => {
    const wrapped = toApiError(new Error('boom'))

    expect(wrapped).toBeInstanceOf(ApiError)
    expect(wrapped.code).toBe('INTERNAL_ERROR')
  })
})
