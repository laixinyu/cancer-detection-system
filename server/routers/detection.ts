import { createTRPCRouter, protectedProcedure } from '../trpc'
import { z } from 'zod'
import { TRPCError } from '@trpc/server'
import { Prisma } from '@prisma/client'
import { assertDetectionTransition } from '@/server/compliance/workflow'
import { writeAuditLog } from '@/server/compliance/audit'
import { buildImageAccessUrl } from '@/lib/storage'

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
      const where: Prisma.DetectionWhereInput = {
        ...(input.status && { status: input.status }),
      }

      if (ctx.session.user.role === 'PATIENT') {
        where.image = { uploadedBy: ctx.session.user.id }
      }

      if (input.priority === 'HIGH') {
        where.cancerProbability = { gte: 0.7 }
      } else if (input.priority === 'MEDIUM') {
        where.AND = [{ cancerProbability: { gte: 0.3 } }, { cancerProbability: { lt: 0.7 } }]
      } else if (input.priority === 'LOW') {
        where.cancerProbability = { lt: 0.3 }
      }

      const detections = await ctx.prisma.detection.findMany({
        where,
        take: input.limit + 1,
        cursor: input.cursor ? { id: input.cursor } : undefined,
        orderBy: input.orderByPriority
          ? [{ cancerProbability: 'desc' }, { createdAt: 'asc' }]
          : { createdAt: 'desc' },
        include: {
          image: {
            include: {
              patient: {
                select: {
                  user: {
                    select: {
                      name: true,
                      email: true,
                    },
                  },
                },
              },
            },
          },
          reviewer: {
            select: {
              name: true,
              email: true,
            },
          },
        },
      })

      let nextCursor: typeof input.cursor | undefined = undefined
      if (detections.length > input.limit) {
        const nextItem = detections.pop()
        nextCursor = nextItem!.id
      }

      return {
        detections: detections.map((detection) => ({
          ...detection,
          image: {
            ...detection.image,
            filePath: buildImageAccessUrl(detection.image.id, detection.image.filePath),
          },
        })),
        nextCursor,
      }
    }),

  getById: protectedProcedure
    .input(z.object({ id: z.string() }))
    .query(async ({ ctx, input }) => {
      const detection = await ctx.prisma.detection.findUnique({
        where: { id: input.id },
        include: {
          image: {
            include: {
              uploader: {
                select: {
                  id: true,
                },
              },
              patient: {
                include: {
                  user: true,
                },
              },
            },
          },
          reviewer: {
            select: {
              name: true,
              email: true,
            },
          },
        },
      })

      if (!detection) {
        throw new TRPCError({ code: 'NOT_FOUND', message: 'Detection not found' })
      }

      if (
        ctx.session.user.role === 'PATIENT' &&
        detection.image.uploadedBy !== ctx.session.user.id
      ) {
        throw new TRPCError({ code: 'FORBIDDEN', message: 'Unauthorized' })
      }

      return {
        ...detection,
        image: {
          ...detection.image,
          filePath: buildImageAccessUrl(detection.image.id, detection.image.filePath),
        },
      }
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
      if (ctx.session.user.role !== 'DOCTOR' && ctx.session.user.role !== 'ADMIN') {
        throw new TRPCError({
          code: 'FORBIDDEN',
          message: 'Only doctors can review detections',
        })
      }

      const existing = await ctx.prisma.detection.findUnique({
        where: { id: input.id },
      })

      if (!existing) {
        throw new TRPCError({ code: 'NOT_FOUND', message: 'Detection not found' })
      }

      assertDetectionTransition(existing.status, input.status)

      const detection = await ctx.prisma.detection.update({
        where: { id: input.id },
        data: {
          status: input.status,
          reviewNotes: input.reviewNotes,
          findings: input.findings || undefined,
          reviewedBy: ctx.session.user.id,
        },
      })

      await writeAuditLog(ctx.prisma, {
        actorUserId: ctx.session.user.id,
        actorRole: ctx.session.user.role,
        action: 'DETECTION_REVIEW',
        entityType: 'detection',
        entityId: detection.id,
        result: 'SUCCESS',
        metadata: {
          fromStatus: existing.status,
          toStatus: input.status,
          hasReviewNotes: Boolean(input.reviewNotes),
        },
      })

      return detection
    }),
})
