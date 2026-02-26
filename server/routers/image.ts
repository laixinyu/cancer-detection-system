import { createTRPCRouter, protectedProcedure } from '../trpc'
import { z } from 'zod'
import { TRPCError } from '@trpc/server'
import { backendRequest } from '@/server/backend-client'

export const imageRouter = createTRPCRouter({
  list: protectedProcedure
    .input(
      z.object({
        limit: z.number().min(1).max(100).default(20),
        cursor: z.string().optional(),
      })
    )
    .query(async ({ ctx, input }) => {
      return backendRequest<{ images: any[]; nextCursor?: string | null }>(ctx, '/images', {
        query: { limit: input.limit, cursor: input.cursor },
      })
    }),

  getById: protectedProcedure
    .input(z.object({ id: z.string() }))
    .query(async ({ ctx, input }) => {
      return backendRequest<any>(ctx, `/images/${input.id}`)
    }),

  delete: protectedProcedure
    .input(z.object({ id: z.string() }))
    .mutation(async () => {
      throw new TRPCError({
        code: 'FORBIDDEN',
        message: 'Hard delete is disabled for compliance and audit integrity',
      })
    }),
})
