import { TRPCError } from '@trpc/server'
import { createTRPCRouter, protectedProcedure } from '../trpc'
import { backendRequest } from '@/server/backend-client'

export const analyticsRouter = createTRPCRouter({
  adminOverview: protectedProcedure.query(async ({ ctx }) => {
    if (ctx.session.user.role !== 'ADMIN') {
      throw new TRPCError({
        code: 'FORBIDDEN',
        message: 'Only admins can access admin analytics',
      })
    }

    return backendRequest<any>(ctx, '/analytics/admin-overview')
  }),
})
