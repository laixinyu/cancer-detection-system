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
  clinicalUse?: string
  heatmapPath?: string
}

export async function requestAiDetection(file: File): Promise<AiDetectionResponse> {
  const aiServiceUrl = process.env.AI_SERVICE_URL || process.env.AI_MODEL_URL

  if (!aiServiceUrl) {
    throw new Error('AI_SERVICE_URL or AI_MODEL_URL is not configured')
  }

  const formData = new FormData()
  formData.append('file', file)

  const response = await fetch(`${aiServiceUrl.replace(/\/$/, '')}/predict`, {
    method: 'POST',
    body: formData,
  })

  if (!response.ok) {
    const message = await response.text()
    throw new Error(`AI service request failed: ${response.status} ${message}`)
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
    clinicalUse: payload.clinicalUse,
    heatmapPath: payload.heatmapPath,
  }
}
