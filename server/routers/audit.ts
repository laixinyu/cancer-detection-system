import { TRPCError } from '@trpc/server'
import { z } from 'zod'
import { createTRPCRouter, protectedProcedure } from '../trpc'
import { backendRequest } from '@/server/backend-client'

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

      return backendRequest<{ logs: any[]; nextCursor?: string | null }>(ctx, '/audits', {
        query: {
          action: input.action,
          entityType: input.entityType,
          entityId: input.entityId,
          result: input.result,
          limit: input.limit,
          cursor: input.cursor,
        },
      })
    }),
})
