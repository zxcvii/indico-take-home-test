import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { errorResponse, jsonResponse } from './test/helpers'

interface Stock {
  item_id: string
  total_stock: number
  reserved_stock: number
  available_stock: number
}

function createFetchMock(seed: Stock) {
  let stock = { ...seed }
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)

    if (url.startsWith('/api/v1/inventory/stock')) {
      return jsonResponse(stock)
    }

    if (url === '/api/v1/inventory/reserve') {
      const body = JSON.parse(String(init?.body)) as { quantity: number }
      stock = {
        ...stock,
        reserved_stock: stock.reserved_stock + body.quantity,
        available_stock: stock.available_stock - body.quantity,
      }
      return jsonResponse(
        {
          status: 'success',
          reservation_id: 'res_1',
          item_id: 'item_4021',
          quantity: body.quantity,
          expires_at: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
        },
        201,
      )
    }

    if (url === '/api/v1/inventory/confirm') {
      return jsonResponse({
        status: 'success',
        reservation_id: 'res_1',
        confirmed_at: new Date().toISOString(),
      })
    }

    return errorResponse('ITEM_NOT_FOUND', 'unknown', 404)
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('App', () => {
  it('runs the reserve then confirm workflow end to end', async () => {
    const user = userEvent.setup()
    vi.stubGlobal(
      'fetch',
      createFetchMock({ item_id: 'item_4021', total_stock: 100, reserved_stock: 10, available_stock: 90 }),
    )

    render(<App />)

    expect(await screen.findByText('100')).toBeInTheDocument()
    expect(screen.getByText('90')).toBeInTheDocument()

    await user.clear(screen.getByLabelText('Quantity'))
    await user.type(screen.getByLabelText('Quantity'), '3')
    await user.click(screen.getByRole('button', { name: 'Reserve' }))

    expect(await screen.findByRole('heading', { name: 'Active Reservation' })).toBeInTheDocument()
    expect(screen.getByText(/^\d{2}:\d{2}$/)).toBeInTheDocument()
    await screen.findByText('13')

    await user.click(screen.getByRole('button', { name: 'Confirm Purchase' }))

    expect(await screen.findByText('Purchase confirmed.')).toBeInTheDocument()
  })

  it('surfaces an insufficient-stock error without creating a reservation', async () => {
    const user = userEvent.setup()
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.startsWith('/api/v1/inventory/stock')) {
          return jsonResponse({ item_id: 'item_4021', total_stock: 100, reserved_stock: 95, available_stock: 5 })
        }
        return errorResponse('INSUFFICIENT_STOCK', 'Only 5 units available, requested 50.', 409, {
          available: 5,
          requested: 50,
        })
      }),
    )

    render(<App />)

    await user.clear(screen.getByLabelText('Quantity'))
    await user.type(screen.getByLabelText('Quantity'), '50')
    await user.click(screen.getByRole('button', { name: 'Reserve' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Only 5 units available, requested 50.')
    expect(screen.queryByRole('heading', { name: 'Active Reservation' })).not.toBeInTheDocument()
  })
})
