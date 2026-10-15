import { useEffect, useState } from 'react'

export interface Countdown {
  remainingMs: number
  formatted: string
  expired: boolean
}

function formatDuration(ms: number): string {
  const totalSeconds = Math.ceil(ms / 1000)
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
}

export function useCountdown(expiresAt: string | null): Countdown {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!expiresAt) {
      return
    }
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [expiresAt])

  const target = expiresAt ? new Date(expiresAt).getTime() : 0
  const remainingMs = expiresAt && !Number.isNaN(target) ? Math.max(0, target - now) : 0

  return {
    remainingMs,
    formatted: formatDuration(remainingMs),
    expired: expiresAt !== null && remainingMs === 0,
  }
}
