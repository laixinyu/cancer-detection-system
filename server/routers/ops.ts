import { TRPCError } from '@trpc/server'
import { z } from 'zod'
import { createTRPCRouter, protectedProcedure } from '../trpc'
import { backendRequest } from '@/server/backend-client'

const deploymentStages = ['RESEARCH_ONLY', 'PILOT_DECISION_SUPPORT', 'CLINICAL_DECISION_SUPPORT'] as const
const regulatoryStatuses = ['NOT_SUBMITTED', 'SUBMITTED', 'APPROVED', 'REJECTED'] as const
const incidentSources = ['AI_SERVICE', 'APP', 'DATABASE', 'PIPELINE', 'SECURITY', 'OTHER'] as const
const incidentSeverities = ['P0', 'P1', 'P2', 'P3'] as const
const incidentStatuses = ['OPEN', 'ACKNOWLEDGED', 'RESOLVED'] as const

function assertAdmin(role: string) {
  if (role !== 'ADMIN') {
    throw new TRPCError({
      code: 'FORBIDDEN',
      message: 'Only admins can access operations endpoints',
    })
  }
}

export const opsRouter = createTRPCRouter({
  readiness: protectedProcedure.query(async ({ ctx }) => {
    assertAdmin(ctx.session.user.role)
    return backendRequest<any>(ctx, '/ops/readiness')
  }),

  dashboard: protectedProcedure.query(async ({ ctx }) => {
    assertAdmin(ctx.session.user.role)
    return backendRequest<any>(ctx, '/ops/dashboard')
  }),

  listEvidence: protectedProcedure
    .input(
      z.object({
        limit: z.number().min(1).max(100).default(20),
      })
    )
    .query(async ({ ctx, input }) => {
      assertAdmin(ctx.session.user.role)
      return backendRequest<any[]>(ctx, '/ops/evidence', {
        query: { limit: input.limit },
      })
    }),

  createEvidence: protectedProcedure
    .input(
      z.object({
        runName: z.string().min(1),
        modelVersion: z.string().min(1),
        datasetName: z.string().min(1),
        datasetVersion: z.string().optional(),
        sampleCount: z.number().int().positive(),
        positiveCount: z.number().int().nonnegative(),
        siteCount: z.number().int().positive(),
        auroc: z.number().min(0).max(1),
        sensitivity: z.number().min(0).max(1),
        specificity: z.number().min(0).max(1),
        ppv: z.number().min(0).max(1).optional(),
        npv: z.number().min(0).max(1).optional(),
        ece: z.number().min(0).max(1).optional(),
        brier: z.number().min(0).max(1).optional(),
        calibrationTemperature: z.number().positive().optional(),
        thresholdHighSensitivity: z.number().min(0).max(1).optional(),
        thresholdHighSpecificity: z.number().min(0).max(1).optional(),
        stageRecommendation: z.enum(deploymentStages).default('RESEARCH_ONLY'),
        regulatoryStatus: z.enum(regulatoryStatuses).default('NOT_SUBMITTED'),
        qaApprovedBy: z.string().optional(),
        medicalApprovedBy: z.string().optional(),
        reportPath: z.string().optional(),
        notes: z.string().optional(),
      })
    )
    .mutation(async ({ ctx, input }) => {
      assertAdmin(ctx.session.user.role)
      return backendRequest<any>(ctx, '/ops/evidence', {
        method: 'POST',
        body: input,
      })
    }),

  listIncidents: protectedProcedure
    .input(
      z.object({
        status: z.enum(incidentStatuses).optional(),
        limit: z.number().min(1).max(100).default(30),
      })
    )
    .query(async ({ ctx, input }) => {
      assertAdmin(ctx.session.user.role)
      return backendRequest<any[]>(ctx, '/ops/incidents', {
        query: { status: input.status, limit: input.limit },
      })
    }),

  createIncident: protectedProcedure
    .input(
      z.object({
        source: z.enum(incidentSources),
        severity: z.enum(incidentSeverities),
        title: z.string().min(1),
        detail: z.string().optional(),
        ownerUserId: z.string().optional(),
      })
    )
    .mutation(async ({ ctx, input }) => {
      assertAdmin(ctx.session.user.role)
      return backendRequest<any>(ctx, '/ops/incidents', {
        method: 'POST',
        body: input,
      })
    }),

  transitionIncident: protectedProcedure
    .input(
      z.object({
        incidentId: z.string().min(1),
        action: z.enum(['ACKNOWLEDGE', 'RESOLVE', 'REOPEN']),
      })
    )
    .mutation(async ({ ctx, input }) => {
      assertAdmin(ctx.session.user.role)
      return backendRequest<any>(ctx, `/ops/incidents/${input.incidentId}/transition`, {
        method: 'POST',
        body: { action: input.action },
      })
    }),
})
