import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { errorResponse, jsonResponse } from '../test/helpers'
import { ReservationForm } from './ReservationForm'

afterEach(() => {
  vi.unstubAllGlobals()
})

const successBody = {
  status: 'success',
  reservation_id: 'res_1',
  item_id: 'item_4021',
  quantity: 2,
  expires_at: '2026-01-01T00:05:00Z',
}

describe('ReservationForm', () => {
  it('submits the reservation and notifies the parent', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(successBody, 201))
    vi.stubGlobal('fetch', fetchMock)
    const onReserved = vi.fn()

    render(<ReservationForm itemId="item_4021" onReserved={onReserved} />)

    await user.clear(screen.getByLabelText('User ID'))
    await user.type(screen.getByLabelText('User ID'), 'usr_1')
    await user.clear(screen.getByLabelText('Quantity'))
    await user.type(screen.getByLabelText('Quantity'), '2')
    await user.click(screen.getByRole('button', { name: 'Reserve' }))

    await waitFor(() => expect(onReserved).toHaveBeenCalledTimes(1))
    expect(onReserved.mock.calls[0][0]).toMatchObject({ reservation_id: 'res_1', quantity: 2 })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/inventory/reserve')
    expect(init).toMatchObject({ method: 'POST' })
    expect(JSON.parse(String(init.body))).toEqual({ user_id: 'usr_1', item_id: 'item_4021', quantity: 2 })
  })

  it('renders the error envelope inline on a non-2xx response', async () => {
    const user = userEvent.setup()
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        errorResponse('INSUFFICIENT_STOCK', 'Only 3 units available, requested 5.', 409, {
          item_id: 'item_4021',
          available: 3,
          requested: 5,
        }),
      ),
    )

    render(<ReservationForm itemId="item_4021" onReserved={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: 'Reserve' }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('Only 3 units available, requested 5.')
    expect(alert).toHaveTextContent('Available: 3')
    expect(alert).toHaveTextContent('Requested: 5')
  })

  it('disables the submit button while the request is in flight', async () => {
    const user = userEvent.setup()
    let resolveFetch: (response: Response) => void = () => {}
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise<Response>((resolve) => {
            resolveFetch = resolve
          }),
      ),
    )

    render(<ReservationForm itemId="item_4021" onReserved={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: 'Reserve' }))

    expect(screen.getByRole('button', { name: 'Reserving…' })).toBeDisabled()

    resolveFetch(jsonResponse(successBody, 201))

    await waitFor(() => expect(screen.getByRole('button', { name: 'Reserve' })).toBeEnabled())
  })
})
