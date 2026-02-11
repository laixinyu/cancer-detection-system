import { createTRPCRouter, protectedProcedure } from '../trpc'
import { z } from 'zod'
import { TRPCError } from '@trpc/server'

export const imageRouter = createTRPCRouter({
  list: protectedProcedure
    .input(
      z.object({
        limit: z.number().min(1).max(100).default(20),
        cursor: z.string().optional(),
      })
    )
    .query(async ({ ctx, input }) => {
      const images = await ctx.prisma.image.findMany({
        where: ctx.session.user.role === 'PATIENT' 
          ? { uploadedBy: ctx.session.user.id }
          : {},
        take: input.limit + 1,
        cursor: input.cursor ? { id: input.cursor } : undefined,
        orderBy: { createdAt: 'desc' },
        include: {
          detections: {
            orderBy: { createdAt: 'desc' },
            take: 1,
            select: {
              id: true,
              createdAt: true,
              cancerProbability: true,
              status: true,
              modelVersion: true,
            },
          },
        },
      })

      let nextCursor: typeof input.cursor | undefined = undefined
      if (images.length > input.limit) {
        const nextItem = images.pop()
        nextCursor = nextItem!.id
      }

      return {
        images,
        nextCursor,
      }
    }),

  getById: protectedProcedure
    .input(z.object({ id: z.string() }))
    .query(async ({ ctx, input }) => {
      const image = await ctx.prisma.image.findUnique({
        where: { id: input.id },
        include: {
          patient: {
            include: {
              user: true,
            },
          },
          detections: {
            include: {
              reviewer: {
                select: {
                  name: true,
                  email: true,
                },
              },
            },
            orderBy: { createdAt: 'desc' },
          },
        },
      })

      if (!image) {
        throw new Error('Image not found')
      }

      // Check permissions
      if (
        ctx.session.user.role === 'PATIENT' &&
        image.uploadedBy !== ctx.session.user.id
      ) {
        throw new Error('Unauthorized')
      }

      return image
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
