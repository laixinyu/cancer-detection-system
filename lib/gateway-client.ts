'use client'

import { buildBackendApiUrl } from '@/lib/backend-api'

type QueryValue = string | number | boolean | undefined | null

function toQuery(params?: Record<string, QueryValue>): string {
  if (!params) {
    return ''
  }
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') {
      continue
    }
    sp.set(k, String(v))
  }
  const q = sp.toString()
  return q ? `?${q}` : ''
}

export async function gatewayGet<T>(
  path: string,
  accessToken: string,
  query?: Record<string, QueryValue>
): Promise<T> {
  const response = await fetch(buildBackendApiUrl(`/api/v1${path}${toQuery(query)}`), {
    method: 'GET',
    headers: {
      Authorization: `Bearer ${accessToken}`,
    },
    cache: 'no-store',
  })
  if (!response.ok) {
    let message = response.statusText
    try {
      const payload = (await response.json()) as { error?: string; hint?: string }
      if (payload.error && payload.hint) {
        message = `${payload.error}: ${payload.hint}`
      } else if (payload.error) {
        message = payload.error
      }
    } catch {
      // Ignore parse failures and keep status text.
    }
    throw new Error(message || 'Gateway request failed')
  }
  return (await response.json()) as T
}
