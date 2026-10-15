import { useState } from 'react'
import type { SubmitEvent } from 'react'
import { reserveStock, toApiError } from '../api/client'
import type { ApiError } from '../api/client'
import type { ReserveResponse } from '../types'

interface ReservationFormProps {
  itemId: string
  onReserved: (reservation: ReserveResponse) => void
}

export function ReservationForm({ itemId, onReserved }: ReservationFormProps) {
  const [userId, setUserId] = useState('usr_9981')
  const [quantity, setQuantity] = useState('1')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<ApiError | null>(null)

  async function handleSubmit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      const reservation = await reserveStock({
        user_id: userId,
        item_id: itemId,
        quantity: Number(quantity),
      })
      onReserved(reservation)
    } catch (err) {
      setError(toApiError(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <section className="panel">
      <h2>Reserve</h2>
      <form className="form" onSubmit={handleSubmit}>
        <label>
          User ID
          <input value={userId} onChange={(event) => setUserId(event.target.value)} required />
        </label>
        <label>
          Quantity
          <input
            type="number"
            min={1}
            value={quantity}
            onChange={(event) => setQuantity(event.target.value)}
            required
          />
        </label>
        <button type="submit" disabled={submitting}>
          {submitting ? 'Reserving…' : 'Reserve'}
        </button>
      </form>
      {error && (
        <div className="error" role="alert">
          <p>{error.message}</p>
          {error.code === 'INSUFFICIENT_STOCK' && error.details && (
            <p className="muted">
              Available: {String(error.details.available ?? '?')} · Requested:{' '}
              {String(error.details.requested ?? '?')}
            </p>
          )}
        </div>
      )}
    </section>
  )
}
