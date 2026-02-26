import { TRPCError } from '@trpc/server'
import { buildBackendApiUrl } from '@/lib/backend-api'

type SessionContext = {
  session: {
    user: {
      accessToken?: string
    }
  } | null
}

type RequestOptions = {
  method?: 'GET' | 'POST' | 'PATCH'
  query?: Record<string, unknown>
  body?: unknown
}

const statusToCode: Record<number, TRPCError['code']> = {
  400: 'BAD_REQUEST',
  401: 'UNAUTHORIZED',
  403: 'FORBIDDEN',
  404: 'NOT_FOUND',
  409: 'CONFLICT',
  429: 'TOO_MANY_REQUESTS',
  503: 'SERVICE_UNAVAILABLE',
}

function toQueryString(query?: Record<string, unknown>): string {
  if (!query) {
    return ''
  }
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === '') {
      continue
    }
    params.set(key, String(value))
  }
  const encoded = params.toString()
  return encoded ? `?${encoded}` : ''
}

function ensureAccessToken(ctx: SessionContext): string {
  const token = ctx.session?.user.accessToken
  if (!token) {
    throw new TRPCError({ code: 'UNAUTHORIZED', message: 'Missing access token' })
  }
  return token
}

async function parseErrorMessage(response: Response): Promise<string> {
  try {
    const payload = (await response.json()) as { error?: string; hint?: string }
    if (payload.error && payload.hint) {
      return `${payload.error}: ${payload.hint}`
    }
    if (payload.error) {
      return payload.error
    }
  } catch {
    // Ignore parse failures and fallback to status text.
  }
  return response.statusText || 'Backend request failed'
}

export async function backendRequest<T>(
  ctx: SessionContext,
  path: string,
  options: RequestOptions = {}
): Promise<T> {
  const token = ensureAccessToken(ctx)
  const method = options.method ?? 'GET'
  const response = await fetch(buildBackendApiUrl(`/api/v1${path}${toQueryString(options.query)}`), {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(options.body !== undefined ? { 'Content-Type': 'application/json' } : {}),
    },
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
    cache: 'no-store',
  })

  if (!response.ok) {
    const message = await parseErrorMessage(response)
    throw new TRPCError({
      code: statusToCode[response.status] ?? 'INTERNAL_SERVER_ERROR',
      message,
    })
  }

  return (await response.json()) as T
}
