import { createTRPCRouter, protectedProcedure } from '../trpc'
import { z } from 'zod'
import { backendRequest } from '@/server/backend-client'

export const reportRouter = createTRPCRouter({
  list: protectedProcedure
    .input(
      z.object({
        patientId: z.string().optional(),
        status: z.enum(['DRAFT', 'FINALIZED']).optional(),
        limit: z.number().min(1).max(100).default(20),
        cursor: z.string().optional(),
      })
    )
    .query(async ({ ctx, input }) => {
      return backendRequest<{ reports: any[]; nextCursor?: string | null }>(ctx, '/reports', {
        query: {
          patientId: input.patientId,
          status: input.status,
          limit: input.limit,
          cursor: input.cursor,
        },
      })
    }),

  getById: protectedProcedure
    .input(z.object({ id: z.string() }))
    .query(async ({ ctx, input }) => {
      return backendRequest<any>(ctx, `/reports/${input.id}`)
    }),

  create: protectedProcedure
    .input(
      z.object({
        detectionId: z.string(),
        patientId: z.string(),
        content: z.any(),
        status: z.enum(['DRAFT', 'FINALIZED']).default('DRAFT'),
      })
    )
    .mutation(async ({ ctx, input }) => {
      return backendRequest<any>(ctx, '/reports', {
        method: 'POST',
        body: input,
      })
    }),

  update: protectedProcedure
    .input(
      z.object({
        id: z.string(),
        content: z.any().optional(),
        status: z.enum(['DRAFT', 'FINALIZED']).optional(),
        pdfPath: z.string().optional(),
      })
    )
    .mutation(async ({ ctx, input }) => {
      const { id, ...payload } = input
      return backendRequest<any>(ctx, `/reports/${id}`, {
        method: 'PATCH',
        body: payload,
      })
    }),
})
