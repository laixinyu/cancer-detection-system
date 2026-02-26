import type { Locale } from '@/lib/i18n'
import type { AiDetectionResponse } from '@/lib/ai-service'

type TaskDecision = {
  highSensitivity: boolean
  highSpecificity: boolean
}

const infectionCoverageLabelMap = {
  infectionAny: {
    en: 'Any Infection Pattern',
    zh: '感染总体风险',
  },
  viralPneumoniaLike: {
    en: 'Viral Pneumonia-like',
    zh: '病毒性肺炎样',
  },
  bacterialPneumoniaLike: {
    en: 'Bacterial Pneumonia-like',
    zh: '细菌性肺炎样',
  },
  covidLikeWhiteLungPattern: {
    en: 'COVID-like White-lung Pattern',
    zh: '类新冠白肺样',
  },
  atypicalInterstitialLike: {
    en: 'Atypical Interstitial-like',
    zh: '非典型间质样',
  },
  pulmonaryEdemaLike: {
    en: 'Pulmonary Edema-like',
    zh: '肺水肿样',
  },
  tbLikePattern: {
    en: 'TB-like Pattern',
    zh: '结核样模式',
  },
} as const

export function getInfectionCoverageLabel(label: string, locale: Locale): string {
  const mapped = infectionCoverageLabelMap[label as keyof typeof infectionCoverageLabelMap]
  if (mapped) {
    return mapped[locale]
  }
  return label
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
  whiteLungScore: number
  overallScore: number
  triagePriority: 'CRITICAL' | 'HIGH' | 'ROUTINE'
  suspectedConditions: string[]
  infectionCoverage: Record<string, number>
  recommendations: string[]
  taskDecisions: Record<string, TaskDecision>
}

export function buildScreeningSummary(ai: AiDetectionResponse): ScreeningSummary {
  const labelScores = ai.labelScores ?? {}
  const pneumoniaScore = pickScore(labelScores, ['肺炎(Pneumonia)', 'Pneumonia'])
  const noduleScore = pickScore(labelScores, ['结节(Nodule)', 'Nodule'])
  const massScore = pickScore(labelScores, ['肿块(Mass)', 'Mass'])
  const lesionScore = clamp01(Math.max(ai.lesionProbability ?? ai.cancerProbability, noduleScore, massScore))
  const whiteLungScore = clamp01(ai.whiteLungAssessment?.whiteLungScore ?? 0)
  const overallScore = clamp01(Math.max(pneumoniaScore, lesionScore, whiteLungScore))
  const infectionCoverage = ai.infectionCoverage ?? {}

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
  if (lesionScore >= 0.3 || lesionDecision.highSensitivity) suspectedConditions.push('LUNG_LESION_RISK')
  if (whiteLungScore >= 0.35) suspectedConditions.push('WHITE_LUNG_PATTERN_RISK')
  if ((infectionCoverage.covidLikeWhiteLungPattern ?? 0) >= 0.35) suspectedConditions.push('COVID_LIKE_INFECTION_RISK')

  let triagePriority: ScreeningSummary['triagePriority'] = 'ROUTINE'
  if (
    lesionDecision.highSpecificity ||
    pneumoniaDecision.highSpecificity ||
    whiteLungScore >= 0.65 ||
    overallScore >= 0.85
  ) {
    triagePriority = 'CRITICAL'
  } else if (
    lesionDecision.highSensitivity ||
    pneumoniaDecision.highSensitivity ||
    whiteLungScore >= 0.45 ||
    overallScore >= 0.6
  ) {
    triagePriority = 'HIGH'
  }

  const recommendations: string[] = []
  if (triagePriority === 'CRITICAL') {
    recommendations.push('PRIORITIZE_RADIOLOGIST_REVIEW')
    recommendations.push('CONSIDER_URGENT_CT_OR_ADDITIONAL_WORKUP')
    recommendations.push('EVALUATE_SEVERE_LUNG_INFECTION_OR_WHITE_LUNG_PATTERN')
  } else if (triagePriority === 'HIGH') {
    recommendations.push('SCHEDULE_PRIORITY_REVIEW')
    recommendations.push('CORRELATE_WITH_CLINICAL_SYMPTOMS_AND_LABS')
  } else {
    recommendations.push('ROUTINE_REVIEW_WORKFLOW')
  }

  return {
    pneumoniaScore: clamp01(pneumoniaScore),
    lesionScore: clamp01(lesionScore),
    whiteLungScore,
    overallScore,
    triagePriority,
    suspectedConditions,
    infectionCoverage,
    recommendations,
    taskDecisions: {
      pneumonia: pneumoniaDecision,
      lesion: lesionDecision,
    },
  }
}
