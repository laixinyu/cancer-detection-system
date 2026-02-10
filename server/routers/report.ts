import { createTRPCRouter, protectedProcedure } from '../trpc'
import { z } from 'zod'
import { TRPCError } from '@trpc/server'
import { Prisma } from '@prisma/client'
import { assertReportTransition } from '@/server/compliance/workflow'
import { writeAuditLog } from '@/server/compliance/audit'

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
      const where: Prisma.ReportWhereInput = {}

      // Patients can only see their own reports
      if (ctx.session.user.role === 'PATIENT') {
        const patient = await ctx.prisma.patient.findUnique({
          where: { userId: ctx.session.user.id },
        })
        if (!patient) {
          return {
            reports: [],
            nextCursor: undefined,
          }
        }
        where.patientId = patient.id
      } else if (input.patientId) {
        where.patientId = input.patientId
      }

      if (input.status) {
        where.status = input.status
      }

      const reports = await ctx.prisma.report.findMany({
        where,
        take: input.limit + 1,
        cursor: input.cursor ? { id: input.cursor } : undefined,
        orderBy: { createdAt: 'desc' },
        include: {
          patient: {
            include: {
              user: {
                select: {
                  name: true,
                  email: true,
                },
              },
            },
          },
          doctor: {
            select: {
              name: true,
              email: true,
            },
          },
          detection: {
            include: {
              image: {
                select: {
                  originalName: true,
                  filePath: true,
                },
              },
            },
          },
        },
      })

      let nextCursor: typeof input.cursor | undefined = undefined
      if (reports.length > input.limit) {
        const nextItem = reports.pop()
        nextCursor = nextItem!.id
      }

      return {
        reports,
        nextCursor,
      }
    }),

  getById: protectedProcedure
    .input(z.object({ id: z.string() }))
    .query(async ({ ctx, input }) => {
      const report = await ctx.prisma.report.findUnique({
        where: { id: input.id },
        include: {
          patient: {
            include: {
              user: true,
            },
          },
          doctor: {
            select: {
              name: true,
              email: true,
            },
          },
          detection: {
            include: {
              image: {
                select: {
                  originalName: true,
                  filePath: true,
                },
              },
            },
          },
        },
      })

      if (!report) {
        throw new TRPCError({ code: 'NOT_FOUND', message: 'Report not found' })
      }

      // Check permissions
      if (ctx.session.user.role === 'PATIENT') {
        const patient = await ctx.prisma.patient.findUnique({
          where: { userId: ctx.session.user.id },
        })
        if (patient?.id !== report.patientId) {
          throw new TRPCError({ code: 'FORBIDDEN', message: 'Unauthorized' })
        }
      }

      return report
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
      if (ctx.session.user.role !== 'DOCTOR' && ctx.session.user.role !== 'ADMIN') {
        throw new TRPCError({
          code: 'FORBIDDEN',
          message: 'Only doctors can create reports',
        })
      }

      const detection = await ctx.prisma.detection.findUnique({
        where: { id: input.detectionId },
        select: {
          id: true,
          status: true,
          image: {
            select: {
              patientId: true,
            },
          },
        },
      })

      if (!detection) {
        throw new TRPCError({ code: 'NOT_FOUND', message: 'Detection not found' })
      }

      if (detection.status !== 'REVIEWED' && detection.status !== 'CONFIRMED') {
        throw new TRPCError({
          code: 'BAD_REQUEST',
          message: 'Detection must be reviewed before report creation',
        })
      }

      if (detection.image.patientId !== input.patientId) {
        throw new TRPCError({
          code: 'BAD_REQUEST',
          message: 'patientId does not match detection patient',
        })
      }

      const existing = await ctx.prisma.report.findFirst({
        where: {
          detectionId: input.detectionId,
        },
      })

      if (existing) {
        const updated = await ctx.prisma.report.update({
          where: { id: existing.id },
          data: {
            content: input.content,
            status: input.status,
            patientId: input.patientId,
            doctorId: ctx.session.user.id,
          },
        })

        await writeAuditLog(ctx.prisma, {
          actorUserId: ctx.session.user.id,
          actorRole: ctx.session.user.role,
          action: 'REPORT_UPSERT',
          entityType: 'report',
          entityId: updated.id,
          result: 'SUCCESS',
          metadata: {
            detectionId: input.detectionId,
            status: updated.status,
          },
        })

        return updated
      }

      const report = await ctx.prisma.report.create({
        data: {
          detectionId: input.detectionId,
          patientId: input.patientId,
          doctorId: ctx.session.user.id,
          content: input.content,
          status: input.status,
        },
      })

      await writeAuditLog(ctx.prisma, {
        actorUserId: ctx.session.user.id,
        actorRole: ctx.session.user.role,
        action: 'REPORT_CREATE',
        entityType: 'report',
        entityId: report.id,
        result: 'SUCCESS',
        metadata: {
          status: report.status,
          detectionId: input.detectionId,
        },
      })

      return report
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
      if (ctx.session.user.role !== 'DOCTOR' && ctx.session.user.role !== 'ADMIN') {
        throw new TRPCError({
          code: 'FORBIDDEN',
          message: 'Only doctors can update reports',
        })
      }

      const { id, ...data } = input

      const existing = await ctx.prisma.report.findUnique({
        where: { id },
        select: { id: true, status: true },
      })

      if (!existing) {
        throw new TRPCError({ code: 'NOT_FOUND', message: 'Report not found' })
      }

      if (data.status) {
        assertReportTransition(existing.status, data.status)
      }

      const report = await ctx.prisma.report.update({
        where: { id },
        data,
      })

      await writeAuditLog(ctx.prisma, {
        actorUserId: ctx.session.user.id,
        actorRole: ctx.session.user.role,
        action: 'REPORT_UPDATE',
        entityType: 'report',
        entityId: report.id,
        result: 'SUCCESS',
        metadata: {
          fromStatus: existing.status,
          toStatus: report.status,
          updatedFields: Object.keys(data),
        },
      })

      return report
    }),
})
