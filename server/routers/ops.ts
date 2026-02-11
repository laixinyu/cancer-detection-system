import { TRPCError } from '@trpc/server'
import { z } from 'zod'
import { createTRPCRouter, protectedProcedure } from '../trpc'

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

function parseFloatEnv(name: string, fallback: number): number {
  const value = process.env[name]
  if (!value) return fallback
  const n = Number.parseFloat(value)
  return Number.isFinite(n) ? n : fallback
}

function parseIntEnv(name: string, fallback: number): number {
  const value = process.env[name]
  if (!value) return fallback
  const n = Number.parseInt(value, 10)
  return Number.isFinite(n) ? n : fallback
}

function evaluateEvidenceGate(evidence: {
  siteCount: number
  auroc: number
  sensitivity: number
  specificity: number
  regulatoryStatus: string
}) {
  const minSiteCount = parseIntEnv('AI_GOV_MIN_SITE_COUNT', 2)
  const minAuroc = parseFloatEnv('AI_GOV_MIN_AUROC', 0.9)
  const minSensitivity = parseFloatEnv('AI_GOV_MIN_SENSITIVITY', 0.9)
  const minSpecificity = parseFloatEnv('AI_GOV_MIN_SPECIFICITY', 0.85)
  const pass =
    evidence.siteCount >= minSiteCount &&
    evidence.auroc >= minAuroc &&
    evidence.sensitivity >= minSensitivity &&
    evidence.specificity >= minSpecificity &&
    (evidence.regulatoryStatus === 'APPROVED')

  return {
    pass,
    criteria: {
      minSiteCount,
      minAuroc,
      minSensitivity,
      minSpecificity,
    },
  }
}

async function getReadiness() {
  const aiServiceUrl = process.env.AI_SERVICE_URL || process.env.AI_MODEL_URL || 'http://localhost:8000'
  const healthUrl = `${aiServiceUrl.replace(/\/$/, '')}/health`
  const start = Date.now()
  try {
    const response = await fetch(healthUrl, { method: 'GET', cache: 'no-store' })
    if (!response.ok) {
      return {
        aiReachable: false,
        aiStatus: `HTTP_${response.status}`,
        aiHealth: null as unknown,
        latencyMs: Date.now() - start,
      }
    }
    const health = (await response.json()) as unknown
    return {
      aiReachable: true,
      aiStatus: 'ok',
      aiHealth: health,
      latencyMs: Date.now() - start,
    }
  } catch (error) {
    return {
      aiReachable: false,
      aiStatus: error instanceof Error ? error.message : 'unknown_error',
      aiHealth: null as unknown,
      latencyMs: Date.now() - start,
    }
  }
}

export const opsRouter = createTRPCRouter({
  readiness: protectedProcedure.query(async ({ ctx }) => {
    assertAdmin(ctx.session.user.role)

    let dbReady = true
    let dbError: string | null = null
    try {
      await ctx.prisma.$queryRaw`SELECT 1`
    } catch (error) {
      dbReady = false
      dbError = error instanceof Error ? error.message : 'db_query_failed'
    }

    const ai = await getReadiness()

    const latestEvidence = await ctx.prisma.clinicalEvidenceRun.findFirst({
      orderBy: { createdAt: 'desc' },
    })
    const evidenceGate = latestEvidence ? evaluateEvidenceGate(latestEvidence) : null

    return {
      timestamp: new Date().toISOString(),
      overallReady: dbReady && ai.aiReachable,
      db: {
        ready: dbReady,
        error: dbError,
      },
      ai,
      evidenceGate,
      latestEvidence,
    }
  }),

  dashboard: protectedProcedure.query(async ({ ctx }) => {
    assertAdmin(ctx.session.user.role)
    const [latestEvidence, openIncidents, p0p1Incidents] = await Promise.all([
      ctx.prisma.clinicalEvidenceRun.findFirst({
        orderBy: { createdAt: 'desc' },
        select: {
          id: true,
          runName: true,
          modelVersion: true,
          auroc: true,
          sensitivity: true,
          specificity: true,
          siteCount: true,
          stageRecommendation: true,
          regulatoryStatus: true,
          createdAt: true,
        },
      }),
      ctx.prisma.opsIncident.count({
        where: {
          status: { in: ['OPEN', 'ACKNOWLEDGED'] },
        },
      }),
      ctx.prisma.opsIncident.count({
        where: {
          severity: { in: ['P0', 'P1'] },
          status: { in: ['OPEN', 'ACKNOWLEDGED'] },
        },
      }),
    ])

    const evidenceGate =
      latestEvidence != null
        ? evaluateEvidenceGate({
            ...latestEvidence,
            regulatoryStatus: latestEvidence.regulatoryStatus,
          })
        : null

    return {
      latestEvidence,
      evidenceGate,
      openIncidents,
      p0p1Incidents,
    }
  }),

  listEvidence: protectedProcedure
    .input(
      z.object({
        limit: z.number().min(1).max(100).default(20),
      })
    )
    .query(async ({ ctx, input }) => {
      assertAdmin(ctx.session.user.role)
      return ctx.prisma.clinicalEvidenceRun.findMany({
        take: input.limit,
        orderBy: { createdAt: 'desc' },
        include: {
          createdBy: {
            select: {
              id: true,
              name: true,
              email: true,
            },
          },
        },
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
      return ctx.prisma.clinicalEvidenceRun.create({
        data: {
          ...input,
          createdByUserId: ctx.session.user.id,
        },
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
      return ctx.prisma.opsIncident.findMany({
        where: input.status ? { status: input.status } : undefined,
        take: input.limit,
        orderBy: { openedAt: 'desc' },
        include: {
          owner: {
            select: {
              id: true,
              name: true,
              email: true,
            },
          },
        },
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
      return ctx.prisma.opsIncident.create({
        data: input,
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

      const existing = await ctx.prisma.opsIncident.findUnique({
        where: { id: input.incidentId },
      })

      if (!existing) {
        throw new TRPCError({
          code: 'NOT_FOUND',
          message: 'Incident not found',
        })
      }

      if (input.action === 'ACKNOWLEDGE') {
        return ctx.prisma.opsIncident.update({
          where: { id: input.incidentId },
          data: {
            status: 'ACKNOWLEDGED',
            acknowledgedAt: new Date(),
            ownerUserId: existing.ownerUserId ?? ctx.session.user.id,
          },
        })
      }

      if (input.action === 'RESOLVE') {
        return ctx.prisma.opsIncident.update({
          where: { id: input.incidentId },
          data: {
            status: 'RESOLVED',
            resolvedAt: new Date(),
          },
        })
      }

      return ctx.prisma.opsIncident.update({
        where: { id: input.incidentId },
        data: {
          status: 'OPEN',
          acknowledgedAt: null,
          resolvedAt: null,
        },
      })
    }),
})
