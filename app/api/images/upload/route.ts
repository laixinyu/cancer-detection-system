import { NextResponse } from 'next/server'
import { getServerSession } from 'next-auth'
import { authOptions } from '@/lib/auth'
import { buildBackendApiUrl } from '@/lib/backend-api'

export async function POST(request: Request) {
  const session = await getServerSession(authOptions)
  if (!session?.user?.accessToken) {
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  }

  const formData = await request.formData()
  const response = await fetch(buildBackendApiUrl('/api/v1/images/upload'), {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${session.user.accessToken}`,
    },
    body: formData,
  })

  const data = await response.json().catch(() => ({ error: 'Upstream error' }))
  return NextResponse.json(data, { status: response.status })
}
