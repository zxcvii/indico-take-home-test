import { act, renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '../api/client'
import { usePolling } from './usePolling'

describe('usePolling', () => {
  it('fetches immediately and exposes the data', async () => {
    const fn = vi.fn().mockResolvedValue('value')

    const { result } = renderHook(() => usePolling(fn, 0))

    expect(fn).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(result.current.data).toBe('value'))
    expect(result.current.error).toBeNull()
    expect(result.current.loading).toBe(false)
  })

  it('polls on the configured interval', async () => {
    const fn = vi.fn().mockResolvedValue(1)

    renderHook(() => usePolling(fn, 30))

    await waitFor(() => expect(fn.mock.calls.length).toBeGreaterThanOrEqual(2))
  })

  it('surfaces failures as an ApiError', async () => {
    const fn = vi.fn().mockRejectedValue(new ApiError('ITEM_NOT_FOUND', 'nope', 404))

    const { result } = renderHook(() => usePolling(fn, 0))

    await waitFor(() => expect(result.current.error?.code).toBe('ITEM_NOT_FOUND'))
    expect(result.current.loading).toBe(false)
  })

  it('fetches again immediately when refresh is called', async () => {
    const fn = vi.fn().mockResolvedValue('x')

    const { result } = renderHook(() => usePolling(fn, 0))
    await waitFor(() => expect(result.current.data).toBe('x'))

    act(() => {
      void result.current.refresh()
    })

    await waitFor(() => expect(fn).toHaveBeenCalledTimes(2))
  })

  it('discards a stale response when the dependencies change', async () => {
    const resolvers: Array<(value: string) => void> = []
    const fn = vi.fn(
      () =>
        new Promise<string>((resolve) => {
          resolvers.push(resolve)
        }),
    )

    const { result, rerender } = renderHook(
      ({ dep }: { dep: number }) => usePolling(fn, 0, [dep]),
      { initialProps: { dep: 1 } },
    )

    expect(fn).toHaveBeenCalledTimes(1)
    rerender({ dep: 2 })
    await waitFor(() => expect(fn).toHaveBeenCalledTimes(2))

    await act(async () => {
      resolvers[1]('fresh')
      await Promise.resolve()
    })
    await waitFor(() => expect(result.current.data).toBe('fresh'))

    await act(async () => {
      resolvers[0]('stale')
      await Promise.resolve()
    })

    expect(result.current.data).toBe('fresh')
  })

  it('stops polling after unmount', async () => {
    const fn = vi.fn().mockResolvedValue(1)

    const { unmount } = renderHook(() => usePolling(fn, 20))
    await waitFor(() => expect(fn.mock.calls.length).toBeGreaterThanOrEqual(1))

    unmount()
    const callsAtUnmount = fn.mock.calls.length
    await new Promise((resolve) => setTimeout(resolve, 70))

    expect(fn.mock.calls.length).toBe(callsAtUnmount)
  })
})
