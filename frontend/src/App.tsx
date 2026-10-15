import { useState } from 'react'
import { ActiveReservationCard } from './components/ActiveReservationCard'
import { InventoryTracker } from './components/InventoryTracker'
import { ReservationForm } from './components/ReservationForm'
import type { ActiveReservation, ReserveResponse } from './types'

const ITEM_ID = 'item_4021'

function App() {
  const [refreshKey, setRefreshKey] = useState(0)
  const [reservation, setReservation] = useState<ActiveReservation | null>(null)

  function handleReserved(res: ReserveResponse) {
    setReservation({
      reservationId: res.reservation_id,
      itemId: res.item_id,
      quantity: res.quantity,
      expiresAt: res.expires_at,
    })
    setRefreshKey((key) => key + 1)
  }

  function handleConfirmed() {
    setRefreshKey((key) => key + 1)
  }

  return (
    <main className="app">
      <header className="app-header">
        <h1>Inventory Reservation Dashboard</h1>
        <p className="muted">Flash-sale stock, reservations and a 5-minute hold.</p>
      </header>
      <InventoryTracker itemId={ITEM_ID} refreshKey={refreshKey} />
      <ReservationForm itemId={ITEM_ID} onReserved={handleReserved} />
      {reservation && (
        <ActiveReservationCard reservation={reservation} onConfirmed={handleConfirmed} />
      )}
    </main>
  )
}

export default App
