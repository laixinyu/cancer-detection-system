import { NextResponse } from 'next/server'
import { z } from 'zod'
import { buildBackendApiUrl } from '@/lib/backend-api'

const registerSchema = z.object({
  name: z.string().min(1),
  email: z.string().email(),
  password: z.string().min(8),
  role: z.enum(['PATIENT']).default('PATIENT'),
  phone: z.string().optional(),
})

export async function POST(request: Request) {
  try {
    const body = await request.json()
    const payload = registerSchema.parse(body)
    const response = await fetch(buildBackendApiUrl('/api/v1/auth/register'), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(payload),
    })

    const data = await response.json().catch(() => ({ error: 'Upstream error' }))
    return NextResponse.json(data, { status: response.status })
  } catch (error) {
    if (error instanceof z.ZodError) {
      return NextResponse.json(
        { error: 'Invalid input', details: error.issues },
        { status: 400 }
      )
    }

    return NextResponse.json(
      { error: 'Internal server error' },
      { status: 500 }
    )
  }
}
