import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { errorResponse, jsonResponse } from '../test/helpers'
import { InventoryTracker } from './InventoryTracker'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('InventoryTracker', () => {
  it('renders the stock breakdown from the API', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse({ item_id: 'item_4021', total_stock: 100, reserved_stock: 15, available_stock: 85 }),
      ),
    )

    render(<InventoryTracker itemId="item_4021" refreshKey={0} />)

    expect(await screen.findByText('100')).toBeInTheDocument()
    expect(screen.getByText('15')).toBeInTheDocument()
    expect(screen.getByText('85')).toBeInTheDocument()
  })

  it('renders the error envelope message when the request fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(errorResponse('ITEM_NOT_FOUND', "Item 'item_x' does not exist.", 404)),
    )

    render(<InventoryTracker itemId="item_x" refreshKey={0} />)

    expect(await screen.findByRole('alert')).toHaveTextContent("Item 'item_x' does not exist.")
  })

  it('refetches the stock when Refresh is clicked', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ item_id: 'item_4021', total_stock: 50, reserved_stock: 20, available_stock: 30 }),
    )
    vi.stubGlobal('fetch', fetchMock)

    render(<InventoryTracker itemId="item_4021" refreshKey={0} />)
    await screen.findByText('50')

    await user.click(screen.getByRole('button', { name: 'Refresh' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
  })
})
