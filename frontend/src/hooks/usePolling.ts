import { useCallback, useEffect, useRef, useState } from 'react'
import { toApiError } from '../api/client'
import type { ApiError } from '../api/client'

export interface PollingResult<T> {
  data: T | null
  error: ApiError | null
  loading: boolean
  refresh: () => void
}

export function usePolling<T>(
  fn: () => Promise<T>,
  intervalMs: number,
  deps: unknown[] = [],
): PollingResult<T> {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<ApiError | null>(null)
  const [loading, setLoading] = useState(true)

  const generation = useRef(0)
  const fnRef = useRef(fn)

  useEffect(() => {
    fnRef.current = fn
  })

  const run = useCallback(async () => {
    const current = ++generation.current
    try {
      const result = await fnRef.current()
      if (generation.current !== current) return
      setData(result)
      setError(null)
    } catch (err) {
      if (generation.current !== current) return
      setError(toApiError(err))
    } finally {
      if (generation.current === current) setLoading(false)
    }
  }, [])

  const depsKey = JSON.stringify(deps)

  useEffect(() => {
    void run()
    const id = intervalMs > 0 ? window.setInterval(() => void run(), intervalMs) : undefined
    return () => {
      if (id !== undefined) window.clearInterval(id)
      generation.current += 1
    }
  }, [intervalMs, run, depsKey])

  return { data, error, loading, refresh: run }
}
