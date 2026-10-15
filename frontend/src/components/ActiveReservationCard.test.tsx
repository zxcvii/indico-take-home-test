import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { errorResponse, jsonResponse } from '../test/helpers'
import type { ActiveReservation } from '../types'
import { ActiveReservationCard } from './ActiveReservationCard'

function activeReservation(overrides: Partial<ActiveReservation> = {}): ActiveReservation {
  return {
    reservationId: 'res_1',
    itemId: 'item_4021',
    quantity: 2,
    expiresAt: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
    ...overrides,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('ActiveReservationCard', () => {
  it('shows the reservation details and a live countdown', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'))

    render(<ActiveReservationCard reservation={activeReservation()} onConfirmed={vi.fn()} />)

    expect(screen.getByText('res_1')).toBeInTheDocument()
    expect(screen.getByText('item_4021')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('05:00')).toBeInTheDocument()
  })

  it('confirms the reservation and notifies the parent', async () => {
    const user = userEvent.setup()
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse({ status: 'success', reservation_id: 'res_1', confirmed_at: '2026-01-01T00:02:00Z' }),
      ),
    )
    const onConfirmed = vi.fn()

    render(<ActiveReservationCard reservation={activeReservation()} onConfirmed={onConfirmed} />)

    await user.click(screen.getByRole('button', { name: 'Confirm Purchase' }))

    expect(await screen.findByText('Purchase confirmed.')).toBeInTheDocument()
    expect(onConfirmed).toHaveBeenCalledTimes(1)
  })

  it('disables confirmation once the reservation has expired', () => {
    const expired = activeReservation({ expiresAt: new Date(Date.now() - 1000).toISOString() })

    render(<ActiveReservationCard reservation={expired} onConfirmed={vi.fn()} />)

    expect(screen.getByText('Expired')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Confirm Purchase' })).toBeDisabled()
  })

  it('maps a 410 expiry response to a friendly message', async () => {
    const user = userEvent.setup()
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(errorResponse('RESERVATION_EXPIRED', 'Reservation expired.', 410)),
    )

    render(<ActiveReservationCard reservation={activeReservation()} onConfirmed={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: 'Confirm Purchase' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('This reservation has expired.')
  })

  it('maps an ALREADY_CONFIRMED response to a friendly message', async () => {
    const user = userEvent.setup()
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(errorResponse('ALREADY_CONFIRMED', 'Already confirmed.', 409)),
    )

    render(<ActiveReservationCard reservation={activeReservation()} onConfirmed={vi.fn()} />)

    await user.click(screen.getByRole('button', { name: 'Confirm Purchase' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('This reservation was already confirmed.')
  })
})
