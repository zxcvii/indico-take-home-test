import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useCountdown } from './useCountdown'

describe('useCountdown', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00.000Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('formats the remaining time as mm:ss', () => {
    const expiresAt = new Date(Date.now() + 5 * 60 * 1000).toISOString()

    const { result } = renderHook(() => useCountdown(expiresAt))

    expect(result.current.formatted).toBe('05:00')
    expect(result.current.expired).toBe(false)
  })

  it('ticks down as time passes', () => {
    const expiresAt = new Date(Date.now() + 5 * 60 * 1000).toISOString()

    const { result } = renderHook(() => useCountdown(expiresAt))

    act(() => {
      vi.advanceTimersByTime(61_000)
    })

    expect(result.current.formatted).toBe('03:59')
    expect(result.current.expired).toBe(false)
  })

  it('reports expiry once the deadline passes', () => {
    const expiresAt = new Date(Date.now() + 2_000).toISOString()

    const { result } = renderHook(() => useCountdown(expiresAt))

    act(() => {
      vi.advanceTimersByTime(3_000)
    })

    expect(result.current.remainingMs).toBe(0)
    expect(result.current.expired).toBe(true)
    expect(result.current.formatted).toBe('00:00')
  })

  it('is inert without an expiry', () => {
    const { result } = renderHook(() => useCountdown(null))

    expect(result.current.remainingMs).toBe(0)
    expect(result.current.expired).toBe(false)
    expect(result.current.formatted).toBe('00:00')
  })
})
