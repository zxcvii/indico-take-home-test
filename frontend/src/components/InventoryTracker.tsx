import { getStock } from '../api/client'
import { usePolling } from '../hooks/usePolling'
import type { StockResponse } from '../types'

interface InventoryTrackerProps {
  itemId: string
  refreshKey: number
}

export function InventoryTracker({ itemId, refreshKey }: InventoryTrackerProps) {
  const { data, error, loading, refresh } = usePolling<StockResponse>(
    () => getStock(itemId),
    3000,
    [itemId, refreshKey],
  )

  return (
    <section className="panel">
      <header className="panel-header">
        <h2>Inventory</h2>
        <button type="button" onClick={refresh} disabled={loading}>
          Refresh
        </button>
      </header>
      <p className="muted">Item: {itemId}</p>
      {error ? (
        <p className="error" role="alert">
          {error.message}
        </p>
      ) : !data ? (
        <p className="muted">Loading…</p>
      ) : (
        <dl className="stock-grid">
          <div>
            <dt>Total</dt>
            <dd>{data.total_stock}</dd>
          </div>
          <div>
            <dt>Reserved</dt>
            <dd>{data.reserved_stock}</dd>
          </div>
          <div>
            <dt>Available</dt>
            <dd>{data.available_stock}</dd>
          </div>
        </dl>
      )}
    </section>
  )
}
