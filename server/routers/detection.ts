import { createTRPCRouter, protectedProcedure } from '../trpc'
import { z } from 'zod'
import { TRPCError } from '@trpc/server'
import { backendRequest } from '@/server/backend-client'

export const detectionRouter = createTRPCRouter({
  list: protectedProcedure
    .input(
      z.object({
        status: z.enum(['PENDING', 'REVIEWED', 'CONFIRMED']).optional(),
        priority: z.enum(['HIGH', 'MEDIUM', 'LOW']).optional(),
        orderByPriority: z.boolean().default(true),
        limit: z.number().min(1).max(100).default(20),
        cursor: z.string().optional(),
      })
    )
    .query(async ({ ctx, input }) => {
      return backendRequest<{ detections: any[]; nextCursor?: string | null }>(ctx, '/detections', {
        query: {
          status: input.status,
          priority: input.priority,
          orderByPriority: input.orderByPriority,
          limit: input.limit,
          cursor: input.cursor,
        },
      })
    }),

  getById: protectedProcedure
    .input(z.object({ id: z.string() }))
    .query(async ({ ctx, input }) => {
      return backendRequest<any>(ctx, `/detections/${input.id}`)
    }),

  review: protectedProcedure
    .input(
      z.object({
        id: z.string(),
        status: z.enum(['REVIEWED', 'CONFIRMED']),
        reviewNotes: z.string().optional(),
        findings: z.unknown().optional(),
      })
    )
    .mutation(async ({ ctx, input }) => {
      return backendRequest<any>(ctx, `/detections/${input.id}/review`, {
        method: 'POST',
        body: {
          status: input.status,
          reviewNotes: input.reviewNotes,
          findings: input.findings,
        },
      })
    }),
})
