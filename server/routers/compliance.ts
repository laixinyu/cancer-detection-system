import { z } from 'zod'
import { createTRPCRouter, protectedProcedure } from '../trpc'
import { backendRequest } from '@/server/backend-client'

const consentInput = z.object({
  consentType: z.enum(['AI_ANALYSIS']).default('AI_ANALYSIS'),
  consentVersion: z.string().min(1).default('v1.0'),
})

export const complianceRouter = createTRPCRouter({
  clinicalScope: protectedProcedure.query(async ({ ctx }) =>
    backendRequest<any>(ctx, '/compliance/clinical-scope')
  ),

  acceptConsent: protectedProcedure
    .input(consentInput)
    .mutation(async ({ ctx, input }) =>
      backendRequest<any>(ctx, '/compliance/consents', {
        method: 'POST',
        body: input,
      })
    ),

  latestConsent: protectedProcedure
    .input(
      z.object({
        consentType: z.enum(['AI_ANALYSIS']).default('AI_ANALYSIS'),
      })
    )
    .query(async ({ ctx, input }) =>
      backendRequest<any>(ctx, '/compliance/consents/latest', {
        query: { consentType: input.consentType },
      })
    ),
})
