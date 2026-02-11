import { NextResponse } from 'next/server'
import { prisma } from '@/lib/prisma'

async function fetchAiHealth() {
  const aiServiceUrl = process.env.AI_SERVICE_URL || process.env.AI_MODEL_URL || 'http://localhost:8000'
  const url = `${aiServiceUrl.replace(/\/$/, '')}/health`
  const startedAt = Date.now()
  try {
    const response = await fetch(url, { method: 'GET', cache: 'no-store' })
    if (!response.ok) {
      return {
        reachable: false,
        status: `HTTP_${response.status}`,
        payload: null as unknown,
        latencyMs: Date.now() - startedAt,
      }
    }
    const payload = (await response.json()) as unknown
    return {
      reachable: true,
      status: 'ok',
      payload,
      latencyMs: Date.now() - startedAt,
    }
  } catch (error) {
    return {
      reachable: false,
      status: error instanceof Error ? error.message : 'ai_unreachable',
      payload: null as unknown,
      latencyMs: Date.now() - startedAt,
    }
  }
}

export async function GET() {
  let dbReady = true
  let dbError: string | null = null

  try {
    await prisma.$queryRaw`SELECT 1`
  } catch (error) {
    dbReady = false
    dbError = error instanceof Error ? error.message : 'database_unreachable'
  }

  const ai = await fetchAiHealth()
  const ready = dbReady && ai.reachable

  return NextResponse.json(
    {
      status: ready ? 'ready' : 'not_ready',
      timestamp: new Date().toISOString(),
      database: {
        ready: dbReady,
        error: dbError,
      },
      ai,
    },
    { status: ready ? 200 : 503 }
  )
}
