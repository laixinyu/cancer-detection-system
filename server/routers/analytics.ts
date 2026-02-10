import { TRPCError } from '@trpc/server'
import { createTRPCRouter, protectedProcedure } from '../trpc'

export const analyticsRouter = createTRPCRouter({
  adminOverview: protectedProcedure.query(async ({ ctx }) => {
    if (ctx.session.user.role !== 'ADMIN') {
      throw new TRPCError({
        code: 'FORBIDDEN',
        message: 'Only admins can access admin analytics',
      })
    }

    const [users, images, detections, reports, recentUsers, recentActivity] = await Promise.all([
      ctx.prisma.user.count(),
      ctx.prisma.image.count(),
      ctx.prisma.detection.count(),
      ctx.prisma.report.count(),
      ctx.prisma.user.findMany({
        orderBy: { createdAt: 'desc' },
        take: 5,
        select: {
          id: true,
          name: true,
          email: true,
          role: true,
          createdAt: true,
        },
      }),
      ctx.prisma.auditLog.findMany({
        orderBy: { createdAt: 'desc' },
        take: 10,
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
      }),
    ])

    const [doctorCount, patientCount, pendingDetections, finalizedReports] = await Promise.all([
      ctx.prisma.user.count({ where: { role: 'DOCTOR' } }),
      ctx.prisma.user.count({ where: { role: 'PATIENT' } }),
      ctx.prisma.detection.count({ where: { status: 'PENDING' } }),
      ctx.prisma.report.count({ where: { status: 'FINALIZED' } }),
    ])

    return {
      stats: {
        users,
        doctors: doctorCount,
        patients: patientCount,
        images,
        detections,
        pendingDetections,
        reports,
        finalizedReports,
      },
      recentUsers,
      recentActivity,
    }
  }),
})
