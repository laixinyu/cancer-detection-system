import { NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'

export async function GET() {
  let dbReady = true
  let dbError: string | null = null

  try {
    await prisma.$queryRaw`SELECT 1`
  } catch (error) {
    dbReady = false
    dbError = error instanceof Error ? error.message : 'database_unreachable'
  }

  return NextResponse.json(
    {
      status: dbReady ? 'ok' : 'degraded',
      service: 'cancer-detection-system',
      timestamp: new Date().toISOString(),
      database: {
        ready: dbReady,
        error: dbError,
      },
    },
    { status: dbReady ? 200 : 503 }
  )
}
