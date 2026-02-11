import type { AiDetectionResponse } from '@/lib/ai-service'

type TaskDecision = {
  highSensitivity: boolean
  highSpecificity: boolean
}

function toNumber(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

function clamp01(value: number): number {
  return Math.max(0, Math.min(1, value))
}

function pickScore(labelScores: Record<string, number> | undefined, keys: string[]): number {
  if (!labelScores) return 0
  const values = keys.map((key) => toNumber(labelScores[key], 0))
  return values.length ? Math.max(...values) : 0
}

export type ScreeningSummary = {
  pneumoniaScore: number
  lesionScore: number
  overallScore: number
  triagePriority: 'CRITICAL' | 'HIGH' | 'ROUTINE'
  suspectedConditions: string[]
  recommendations: string[]
  taskDecisions: Record<string, TaskDecision>
}

export function buildScreeningSummary(ai: AiDetectionResponse): ScreeningSummary {
  const labelScores = ai.labelScores ?? {}
  const pneumoniaScore = pickScore(labelScores, ['肺炎(Pneumonia)', 'Pneumonia'])
  const noduleScore = pickScore(labelScores, ['结节(Nodule)', 'Nodule'])
  const massScore = pickScore(labelScores, ['肿块(Mass)', 'Mass'])
  const opacityScore = pickScore(labelScores, ['浸润/实变(Opacity)', 'Lung Opacity', 'Opacity'])
  const lesionScore = clamp01(Math.max(ai.cancerProbability, noduleScore, massScore, opacityScore))
  const overallScore = clamp01(Math.max(pneumoniaScore, lesionScore))

  const pneumoniaDecision = ai.taskDecisions?.pneumonia ?? {
    highSensitivity: false,
    highSpecificity: false,
  }
  const lesionDecision = ai.taskDecisions?.lesion ?? {
    highSensitivity: ai.decisionHighSensitivity ?? false,
    highSpecificity: ai.decisionHighSpecificity ?? false,
  }

  const suspectedConditions: string[] = []
  if (pneumoniaScore >= 0.3 || pneumoniaDecision.highSensitivity) suspectedConditions.push('PNEUMONIA_RISK')
  if (lesionScore >= 0.3 || lesionDecision.highSensitivity) suspectedConditions.push('LUNG_CANCER_RELATED_LESION_RISK')

  let triagePriority: ScreeningSummary['triagePriority'] = 'ROUTINE'
  if (
    lesionDecision.highSpecificity ||
    pneumoniaDecision.highSpecificity ||
    overallScore >= 0.85
  ) {
    triagePriority = 'CRITICAL'
  } else if (
    lesionDecision.highSensitivity ||
    pneumoniaDecision.highSensitivity ||
    overallScore >= 0.6
  ) {
    triagePriority = 'HIGH'
  }

  const recommendations: string[] = []
  if (triagePriority === 'CRITICAL') {
    recommendations.push('PRIORITIZE_RADIOLOGIST_REVIEW')
    recommendations.push('CONSIDER_URGENT_CT_OR_ADDITIONAL_WORKUP')
  } else if (triagePriority === 'HIGH') {
    recommendations.push('SCHEDULE_PRIORITY_REVIEW')
    recommendations.push('CORRELATE_WITH_CLINICAL_SYMPTOMS_AND_LABS')
  } else {
    recommendations.push('ROUTINE_REVIEW_WORKFLOW')
  }

  return {
    pneumoniaScore: clamp01(pneumoniaScore),
    lesionScore: clamp01(lesionScore),
    overallScore,
    triagePriority,
    suspectedConditions,
    recommendations,
    taskDecisions: {
      pneumonia: pneumoniaDecision,
      lesion: lesionDecision,
    },
  }
}
