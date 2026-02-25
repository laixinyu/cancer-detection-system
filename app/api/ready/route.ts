import { NextResponse } from 'next/server'
import { buildBackendApiUrl } from '@/lib/backend-api'

export async function GET() {
  const response = await fetch(buildBackendApiUrl('/ready'), {
    method: 'GET',
    cache: 'no-store',
  })
  const data = await response.json().catch(() => ({ status: 'not_ready' }))
  return NextResponse.json(data, { status: response.status })
}
