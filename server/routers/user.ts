import { createTRPCRouter, protectedProcedure } from '../trpc'
import { z } from 'zod'
import { backendRequest } from '@/server/backend-client'

export const userRouter = createTRPCRouter({
  me: protectedProcedure.query(async ({ ctx }) => backendRequest<any>(ctx, '/users/me')),

  updateProfile: protectedProcedure
    .input(
      z.object({
        name: z.string().min(1).optional(),
        phone: z.string().optional(),
      })
    )
    .mutation(async ({ ctx, input }) =>
      backendRequest<any>(ctx, '/users/me', {
        method: 'PATCH',
        body: input,
      })
    ),
})
