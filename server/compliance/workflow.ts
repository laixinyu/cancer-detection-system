import { TRPCError } from '@trpc/server'

export const CLINICAL_SCOPE = {
  indication: '胸部X光肺部癌症辅助筛查',
  outOfScope: [
    '非胸部X光影像',
    '儿科病例',
    '急诊替代诊断',
    '仅凭AI结果出具最终诊断',
  ],
  highRiskThreshold: 0.75,
  productPositioning: '仅作医生辅助决策，不可替代临床诊断',
} as const

type DetectionStatus = 'PENDING' | 'REVIEWED' | 'CONFIRMED'
type ReportStatus = 'DRAFT' | 'FINALIZED'

const detectionTransitions: Record<DetectionStatus, DetectionStatus[]> = {
  PENDING: ['REVIEWED', 'CONFIRMED'],
  REVIEWED: ['CONFIRMED'],
  CONFIRMED: [],
}

const reportTransitions: Record<ReportStatus, ReportStatus[]> = {
  DRAFT: ['FINALIZED'],
  FINALIZED: [],
}

export function assertDetectionTransition(
  from: DetectionStatus,
  to: DetectionStatus
): void {
  if (from === to) {
    return
  }

  if (!detectionTransitions[from].includes(to)) {
    throw new TRPCError({
      code: 'BAD_REQUEST',
      message: `Invalid detection status transition: ${from} -> ${to}`,
    })
  }
}

export function assertReportTransition(from: ReportStatus, to: ReportStatus): void {
  if (from === to) {
    return
  }

  if (!reportTransitions[from].includes(to)) {
    throw new TRPCError({
      code: 'BAD_REQUEST',
      message: `Invalid report status transition: ${from} -> ${to}`,
    })
  }
}
