import { NextResponse } from 'next/server'
import { getServerSession } from 'next-auth'
import { authOptions } from '@/lib/auth'
import { buildBackendApiUrl } from '@/lib/backend-api'

export async function GET(
  _request: Request,
  context: { params: Promise<{ id: string }> }
) {
  const session = await getServerSession(authOptions)
  if (!session?.user?.accessToken) {
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  }

  const { id } = await context.params
  const response = await fetch(
    buildBackendApiUrl(`/api/v1/images/${id}/file`),
    {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${session.user.accessToken}`,
      },
    }
  )

  if (!response.ok) {
    const data = await response.json().catch(() => ({ error: 'File unavailable' }))
    return NextResponse.json(data, { status: response.status })
  }

  const blob = await response.arrayBuffer()
  return new NextResponse(blob, {
    status: 200,
    headers: {
      'Content-Type':
        response.headers.get('content-type') || 'application/octet-stream',
      'Cache-Control':
        response.headers.get('cache-control') || 'private, max-age=60',
      'Content-Disposition':
        response.headers.get('content-disposition') || 'inline',
    },
  })
}
