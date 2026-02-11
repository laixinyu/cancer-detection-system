export interface AiRegion {
  x: number
  y: number
  width: number
  height: number
  confidence: number
  label: string
}

export interface AiDetectionResponse {
  modelVersion: string
  cancerProbability: number
  regions: AiRegion[]
  labelScores?: Record<string, number>
  topFindings?: string[]
  calibrationTemperature?: number
  decisionHighSensitivity?: boolean
  decisionHighSpecificity?: boolean
  taskDecisions?: Record<string, { highSensitivity: boolean; highSpecificity: boolean }>
  operatingPointsUsed?: Record<string, { highSensitivityThreshold: number; highSpecificityThreshold: number }>
  clinicalUse?: string
  clinicalStage?: string
  detectorModelLoaded?: boolean
  heatmapPath?: string
}

const DEFAULT_AI_TIMEOUT_MS = 20000
const DEFAULT_AI_RETRIES = 1

function parsePositiveInt(value: string | undefined, fallback: number): number {
  const parsed = Number.parseInt(value ?? '', 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

export async function requestAiDetection(file: File): Promise<AiDetectionResponse> {
  const aiServiceUrl = process.env.AI_SERVICE_URL || process.env.AI_MODEL_URL

  if (!aiServiceUrl) {
    throw new Error('AI_SERVICE_URL or AI_MODEL_URL is not configured')
  }

  const timeoutMs = parsePositiveInt(process.env.AI_REQUEST_TIMEOUT_MS, DEFAULT_AI_TIMEOUT_MS)
  const retries = parsePositiveInt(process.env.AI_REQUEST_RETRIES, DEFAULT_AI_RETRIES)
  const endpoint = `${aiServiceUrl.replace(/\/$/, '')}/predict`
  let response: Response | null = null
  let lastError: Error | null = null

  for (let attempt = 0; attempt <= retries; attempt++) {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), timeoutMs)
    try {
      const formData = new FormData()
      formData.append('file', file)
      response = await fetch(endpoint, {
        method: 'POST',
        body: formData,
        signal: controller.signal,
      })

      if (response.ok) {
        break
      }

      const message = await response.text()
      if (response.status >= 500 && attempt < retries) {
        continue
      }
      throw new Error(`AI service request failed: ${response.status} ${message}`)
    } catch (error) {
      const normalized =
        error instanceof Error ? error : new Error(`Unknown AI service error: ${String(error)}`)
      lastError = normalized
      if (attempt >= retries) {
        throw normalized
      }
    } finally {
      clearTimeout(timer)
    }
  }

  if (!response || !response.ok) {
    throw lastError ?? new Error('AI service request failed without response')
  }

  const payload = (await response.json()) as Partial<AiDetectionResponse>

  if (
    typeof payload.cancerProbability !== 'number' ||
    !Array.isArray(payload.regions) ||
    typeof payload.modelVersion !== 'string'
  ) {
    throw new Error('Invalid AI service response payload')
  }

  return {
    modelVersion: payload.modelVersion,
    cancerProbability: Math.max(0, Math.min(1, payload.cancerProbability)),
    regions: payload.regions,
    labelScores: payload.labelScores,
    topFindings: payload.topFindings,
    calibrationTemperature: payload.calibrationTemperature,
    decisionHighSensitivity: payload.decisionHighSensitivity,
    decisionHighSpecificity: payload.decisionHighSpecificity,
    taskDecisions: payload.taskDecisions,
    operatingPointsUsed: payload.operatingPointsUsed,
    clinicalUse: payload.clinicalUse,
    clinicalStage: payload.clinicalStage,
    detectorModelLoaded: payload.detectorModelLoaded,
    heatmapPath: payload.heatmapPath,
  }
}
