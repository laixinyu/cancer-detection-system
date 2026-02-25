import { NextResponse } from 'next/server'
import { buildBackendApiUrl } from '@/lib/backend-api'

export async function GET() {
  const response = await fetch(buildBackendApiUrl('/health'), {
    method: 'GET',
    cache: 'no-store',
  })
  const data = await response.json().catch(() => ({ status: 'degraded' }))
  return NextResponse.json(data, { status: response.status })
}
