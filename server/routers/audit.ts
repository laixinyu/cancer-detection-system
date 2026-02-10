import { TRPCError } from '@trpc/server'
import { Prisma } from '@prisma/client'
import { z } from 'zod'
import { createTRPCRouter, protectedProcedure } from '../trpc'

export const auditRouter = createTRPCRouter({
  list: protectedProcedure
    .input(
      z.object({
        action: z.string().optional(),
        entityType: z.string().optional(),
        entityId: z.string().optional(),
        result: z.enum(['SUCCESS', 'FAILED']).optional(),
        limit: z.number().min(1).max(100).default(20),
        cursor: z.string().optional(),
      })
    )
    .query(async ({ ctx, input }) => {
      if (ctx.session.user.role !== 'ADMIN' && ctx.session.user.role !== 'DOCTOR') {
        throw new TRPCError({
          code: 'FORBIDDEN',
          message: 'Only doctors and admins can view audit logs',
        })
      }

      const where: Prisma.AuditLogWhereInput = {
        ...(input.action && { action: input.action }),
        ...(input.entityType && { entityType: input.entityType }),
        ...(input.entityId && { entityId: input.entityId }),
        ...(input.result && { result: input.result }),
      }

      const logs = await ctx.prisma.auditLog.findMany({
        where,
        take: input.limit + 1,
        cursor: input.cursor ? { id: input.cursor } : undefined,
        orderBy: { createdAt: 'desc' },
        include: {
          actorUser: {
            select: {
              id: true,
              name: true,
              email: true,
              role: true,
            },
          },
        },
      })

      let nextCursor: string | undefined
      if (logs.length > input.limit) {
        const nextItem = logs.pop()
        nextCursor = nextItem?.id
      }

      return {
        logs,
        nextCursor,
      }
    }),
})
