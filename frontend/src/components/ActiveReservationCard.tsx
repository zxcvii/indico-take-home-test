import { useState } from 'react'
import { confirmReservation, toApiError } from '../api/client'
import { useCountdown } from '../hooks/useCountdown'
import type { ActiveReservation } from '../types'

interface ActiveReservationCardProps {
  reservation: ActiveReservation
  onConfirmed: () => void
}

function mapConfirmError(err: unknown): string {
  const apiError = toApiError(err)
  switch (apiError.code) {
    case 'RESERVATION_EXPIRED':
      return 'This reservation has expired.'
    case 'ALREADY_CONFIRMED':
      return 'This reservation was already confirmed.'
    case 'RESERVATION_NOT_FOUND':
      return 'Reservation not found.'
    default:
      return apiError.message
  }
}

export function ActiveReservationCard({ reservation, onConfirmed }: ActiveReservationCardProps) {
  const { formatted, expired } = useCountdown(reservation.expiresAt)
  const [confirming, setConfirming] = useState(false)
  const [confirmed, setConfirmed] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleConfirm() {
    setConfirming(true)
    setError(null)
    try {
      await confirmReservation({ reservation_id: reservation.reservationId })
      setConfirmed(true)
      onConfirmed()
    } catch (err) {
      setError(mapConfirmError(err))
    } finally {
      setConfirming(false)
    }
  }

  return (
    <section className="panel">
      <header className="panel-header">
        <h2>Active Reservation</h2>
        <span className={expired ? 'badge badge-expired' : 'badge badge-active'}>
          {expired ? 'Expired' : formatted}
        </span>
      </header>
      <dl className="stock-grid">
        <div>
          <dt>Reservation</dt>
          <dd>{reservation.reservationId}</dd>
        </div>
        <div>
          <dt>Item</dt>
          <dd>{reservation.itemId}</dd>
        </div>
        <div>
          <dt>Quantity</dt>
          <dd>{reservation.quantity}</dd>
        </div>
      </dl>
      {confirmed ? (
        <p className="success">Purchase confirmed.</p>
      ) : (
        <>
          <button type="button" onClick={handleConfirm} disabled={confirming || expired}>
            {confirming ? 'Confirming…' : 'Confirm Purchase'}
          </button>
          {error && (
            <p className="error" role="alert">
              {error}
            </p>
          )}
        </>
      )}
    </section>
  )
}
