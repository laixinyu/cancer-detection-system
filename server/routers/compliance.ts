import { TRPCError } from '@trpc/server'
import { z } from 'zod'
import { createTRPCRouter, protectedProcedure } from '../trpc'
import { CLINICAL_SCOPE } from '@/server/compliance/workflow'
import { writeAuditLog } from '@/server/compliance/audit'

const consentInput = z.object({
  consentType: z.enum(['AI_ANALYSIS']).default('AI_ANALYSIS'),
  consentVersion: z.string().min(1).default('v1.0'),
})

export const complianceRouter = createTRPCRouter({
  clinicalScope: protectedProcedure.query(() => {
    return CLINICAL_SCOPE
  }),

  acceptConsent: protectedProcedure
    .input(consentInput)
    .mutation(async ({ ctx, input }) => {
      let patient = await ctx.prisma.patient.findUnique({
        where: { userId: ctx.session.user.id },
      })

      if (!patient) {
        if (ctx.session.user.role !== 'PATIENT') {
          throw new TRPCError({
            code: 'BAD_REQUEST',
            message: 'Only patient accounts can self-register consent',
          })
        }

        patient = await ctx.prisma.patient.create({
          data: {
            userId: ctx.session.user.id,
            dateOfBirth: new Date('1990-01-01'),
            gender: 'OTHER',
          },
        })
      }

      const consent = await ctx.prisma.patientConsent.upsert({
        where: {
          patientId_consentType_consentVersion: {
            patientId: patient.id,
            consentType: input.consentType,
            consentVersion: input.consentVersion,
          },
        },
        create: {
          patientId: patient.id,
          consentType: input.consentType,
          consentVersion: input.consentVersion,
          accepted: true,
          acceptedByUserId: ctx.session.user.id,
        },
        update: {
          accepted: true,
          acceptedAt: new Date(),
          acceptedByUserId: ctx.session.user.id,
        },
      })

      await writeAuditLog(ctx.prisma, {
        actorUserId: ctx.session.user.id,
        actorRole: ctx.session.user.role,
        action: 'CONSENT_ACCEPT',
        entityType: 'patient_consent',
        entityId: consent.id,
        result: 'SUCCESS',
        metadata: {
          consentType: input.consentType,
          consentVersion: input.consentVersion,
        },
      })

      return consent
    }),

  latestConsent: protectedProcedure
    .input(
      z.object({
        consentType: z.enum(['AI_ANALYSIS']).default('AI_ANALYSIS'),
      })
    )
    .query(async ({ ctx, input }) => {
      const patient = await ctx.prisma.patient.findUnique({
        where: { userId: ctx.session.user.id },
      })

      if (!patient) {
        return null
      }

      return ctx.prisma.patientConsent.findFirst({
        where: {
          patientId: patient.id,
          consentType: input.consentType,
          accepted: true,
        },
        orderBy: {
          acceptedAt: 'desc',
        },
      })
    }),
})
