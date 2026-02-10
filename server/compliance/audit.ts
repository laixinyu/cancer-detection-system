import type { PrismaClient } from '@prisma/client'
import { Prisma } from '@prisma/client'

interface AuditPayload {
  actorUserId?: string
  actorRole?: string
  action: string
  entityType: string
  entityId: string
  result: 'SUCCESS' | 'FAILED'
  metadata?: unknown
}

export async function writeAuditLog(
  prisma: PrismaClient,
  payload: AuditPayload
): Promise<void> {
  try {
    await prisma.auditLog.create({
      data: {
        actorUserId: payload.actorUserId,
        actorRole: payload.actorRole,
        action: payload.action,
        entityType: payload.entityType,
        entityId: payload.entityId,
        result: payload.result,
        metadata: payload.metadata as Prisma.InputJsonValue | undefined,
      },
    })
  } catch (error) {
    const reason = error instanceof Error ? error.message : String(error)
    throw new Error(`Audit log write failed: ${reason}`)
  }
}
