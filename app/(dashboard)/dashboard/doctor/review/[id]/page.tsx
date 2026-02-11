'use client'

import { useMemo, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import ImageViewer from '@/components/image-viewer/image-viewer'
import { api } from '@/lib/trpc'
import { useI18n } from '@/components/i18n-provider'
import { getInfectionCoverageLabel } from '@/lib/screening'

type AnnotationRect = {
  id: string
  x: number
  y: number
  width: number
  height: number
  label: string
  confidence?: number
  color: string
}

function calculateAge(dob: Date | string) {
  const birth = new Date(dob)
  const today = new Date()
  let age = today.getFullYear() - birth.getFullYear()
  const monthDiff = today.getMonth() - birth.getMonth()
  if (monthDiff < 0 || (monthDiff === 0 && today.getDate() < birth.getDate())) {
    age--
  }
  return age
}

function getRiskLevel(probability: number, isZh: boolean) {
  if (probability < 0.3) return { text: isZh ? '低风险' : 'Low Risk', color: 'bg-green-100 text-green-700' }
  if (probability < 0.7) return { text: isZh ? '中风险' : 'Medium Risk', color: 'bg-yellow-100 text-yellow-700' }
  return { text: isZh ? '高风险' : 'High Risk', color: 'bg-red-100 text-red-700' }
}

function extractAnnotations(findings: unknown): AnnotationRect[] {
  if (!findings || typeof findings !== 'object') {
    return []
  }

  const regions = (findings as { regions?: unknown }).regions
  if (!Array.isArray(regions)) {
    return []
  }

  const annotations: AnnotationRect[] = []

  regions.forEach((region, index) => {
    if (!region || typeof region !== 'object') {
      return
    }

    const r = region as Record<string, unknown>
    if (
      typeof r.x !== 'number' ||
      typeof r.y !== 'number' ||
      typeof r.width !== 'number' ||
      typeof r.height !== 'number'
    ) {
      return
    }

    annotations.push({
      id: `ann-${index}`,
      x: r.x,
      y: r.y,
      width: r.width,
      height: r.height,
      label: typeof r.label === 'string' ? r.label : 'Finding',
      confidence: typeof r.confidence === 'number' ? r.confidence : undefined,
      color: '#ef4444',
    })
  })

  return annotations
}

function extractTopFindings(findings: unknown): string[] {
  if (!findings || typeof findings !== 'object') {
    return []
  }

  const topFindings = (findings as { topFindings?: unknown }).topFindings
  if (!Array.isArray(topFindings)) {
    return []
  }

  return topFindings.filter((item): item is string => typeof item === 'string')
}

function extractLabelScores(findings: unknown): Record<string, number> {
  if (!findings || typeof findings !== 'object') {
    return {}
  }

  const labelScores = (findings as { labelScores?: unknown }).labelScores
  if (!labelScores || typeof labelScores !== 'object') {
    return {}
  }

  const scores: Record<string, number> = {}
  Object.entries(labelScores as Record<string, unknown>).forEach(([label, value]) => {
    if (typeof value === 'number') {
      scores[label] = value
    }
  })

  return scores
}

function extractBooleanField(findings: unknown, field: string, fallback = false): boolean {
  if (!findings || typeof findings !== 'object') {
    return fallback
  }
  const value = (findings as Record<string, unknown>)[field]
  return typeof value === 'boolean' ? value : fallback
}

function extractNumberField(findings: unknown, field: string): number | null {
  if (!findings || typeof findings !== 'object') {
    return null
  }
  const value = (findings as Record<string, unknown>)[field]
  return typeof value === 'number' ? value : null
}

function extractStringField(findings: unknown, field: string, fallback = ''): string {
  if (!findings || typeof findings !== 'object') {
    return fallback
  }
  const value = (findings as Record<string, unknown>)[field]
  return typeof value === 'string' ? value : fallback
}

function extractScreeningSummary(findings: unknown): {
  pneumoniaScore?: number
  lesionScore?: number
  whiteLungScore?: number
  overallScore?: number
  triagePriority?: string
  suspectedConditions?: string[]
  infectionCoverage?: Record<string, number>
  recommendations?: string[]
} | null {
  if (!findings || typeof findings !== 'object') return null
  const summary = (findings as { screeningSummary?: unknown }).screeningSummary
  if (!summary || typeof summary !== 'object') return null
  const s = summary as Record<string, unknown>
  return {
    pneumoniaScore: typeof s.pneumoniaScore === 'number' ? s.pneumoniaScore : undefined,
    lesionScore: typeof s.lesionScore === 'number' ? s.lesionScore : undefined,
    whiteLungScore: typeof s.whiteLungScore === 'number' ? s.whiteLungScore : undefined,
    overallScore: typeof s.overallScore === 'number' ? s.overallScore : undefined,
    triagePriority: typeof s.triagePriority === 'string' ? s.triagePriority : undefined,
    suspectedConditions: Array.isArray(s.suspectedConditions)
      ? s.suspectedConditions.filter((v): v is string => typeof v === 'string')
      : undefined,
    infectionCoverage:
      s.infectionCoverage && typeof s.infectionCoverage === 'object'
        ? (s.infectionCoverage as Record<string, number>)
        : undefined,
    recommendations: Array.isArray(s.recommendations)
      ? s.recommendations.filter((v): v is string => typeof v === 'string')
      : undefined,
  }
}

export default function ReviewPage() {
  const { locale } = useI18n()
  const isZh = locale === 'zh'
  const router = useRouter()
  const params = useParams<{ id: string }>()
  const detectionId = params.id

  const [status, setStatus] = useState<'REVIEWED' | 'CONFIRMED'>('REVIEWED')
  const [notes, setNotes] = useState('')

  const detectionQuery = api.detection.getById.useQuery(
    { id: detectionId },
    { enabled: Boolean(detectionId) }
  )

  const reviewMutation = api.detection.review.useMutation({
    onSuccess: () => {},
  })
  const reportUpsertMutation = api.report.create.useMutation()

  const detection = detectionQuery.data
  const risk = getRiskLevel(detection?.cancerProbability ?? 0, isZh)
  const annotations = useMemo(
    () => extractAnnotations(detection?.findings),
    [detection?.findings]
  )
  const topFindings = useMemo(
    () => extractTopFindings(detection?.findings),
    [detection?.findings]
  )
  const labelScores = useMemo(
    () => extractLabelScores(detection?.findings),
    [detection?.findings]
  )
  const scoreEntries = useMemo(
    () => Object.entries(labelScores).sort((a, b) => b[1] - a[1]),
    [labelScores]
  )
  const decisionHighSensitivity = useMemo(
    () => extractBooleanField(detection?.findings, 'decisionHighSensitivity', false),
    [detection?.findings]
  )
  const decisionHighSpecificity = useMemo(
    () => extractBooleanField(detection?.findings, 'decisionHighSpecificity', false),
    [detection?.findings]
  )
  const calibrationTemperature = useMemo(
    () => extractNumberField(detection?.findings, 'calibrationTemperature'),
    [detection?.findings]
  )
  const clinicalUse = useMemo(
    () => extractStringField(detection?.findings, 'clinicalUse', 'RESEARCH_ONLY'),
    [detection?.findings]
  )
  const screeningSummary = useMemo(
    () => extractScreeningSummary(detection?.findings),
    [detection?.findings]
  )

  const handleSubmit = async () => {
    if (!detectionId || !notes.trim() || !detection) {
      return
    }

    await reviewMutation.mutateAsync({
      id: detectionId,
      status,
      reviewNotes: notes,
    })

    const diagnosis =
      status === 'CONFIRMED'
        ? isZh
          ? '医生已确认：存在需进一步临床评估的可疑病灶'
          : 'Confirmed by doctor: suspicious lesion requires further clinical evaluation'
        : isZh
        ? '医生已复核：建议继续观察并结合临床信息判断'
        : 'Reviewed by doctor: continue observation and correlate with clinical context'

    await reportUpsertMutation.mutateAsync({
      detectionId: detection.id,
      patientId: detection.image.patient.id,
      status: status === 'CONFIRMED' ? 'FINALIZED' : 'DRAFT',
      content: {
        diagnosis,
        findings: topFindings,
        labelScores,
        screeningSummary,
        decisionHighSensitivity,
        decisionHighSpecificity,
        calibrationTemperature,
        clinicalUse,
        doctorNotes: notes,
        aiCancerProbability: detection.cancerProbability,
      },
    })

    router.push('/dashboard/doctor/queue')
  }

  if (detectionQuery.isLoading) {
    return <div className="p-6 text-gray-500">{isZh ? '正在加载检测结果...' : 'Loading detection...'}</div>
  }

  if (detectionQuery.error || !detection) {
    return (
      <div className="p-6 text-red-600">
        {detectionQuery.error?.message ?? (isZh ? '未找到检测记录' : 'Detection not found')}
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">{isZh ? '检测审阅' : 'Review Detection'}</h1>
          <p className="text-gray-600 mt-2">{isZh ? '患者：' : 'Patient: '}{detection.image.patient.user.name}</p>
        </div>
        <Button variant="outline" onClick={() => router.back()}>
          {isZh ? '返回队列' : 'Back to Queue'}
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{isZh ? '患者信息' : 'Patient Information'}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid md:grid-cols-4 gap-4">
            <div>
              <div className="text-sm text-gray-600">{isZh ? '姓名' : 'Name'}</div>
              <div className="font-medium">{detection.image.patient.user.name}</div>
            </div>
            <div>
              <div className="text-sm text-gray-600">{isZh ? '年龄 / 性别' : 'Age / Gender'}</div>
              <div className="font-medium">
                {calculateAge(detection.image.patient.dateOfBirth)} / {detection.image.patient.gender}
              </div>
            </div>
            <div>
              <div className="text-sm text-gray-600">{isZh ? '邮箱' : 'Email'}</div>
              <div className="font-medium">{detection.image.patient.user.email}</div>
            </div>
            <div>
              <div className="text-sm text-gray-600">{isZh ? '上传日期' : 'Upload Date'}</div>
              <div className="font-medium">
                {new Date(detection.image.createdAt).toLocaleDateString()}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-6">
          <div className="rounded-md border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900">
            <div className="font-semibold">{isZh ? '非临床用途声明' : 'Non-clinical Use Notice'}</div>
            <div className="mt-1">
              {isZh
                ? '当前模型为研究验证用途，仅提供风险参考，不能替代放射科医生诊断。'
                : 'Current model output is for research validation only and must not replace radiologist diagnosis.'}
            </div>
            <div className="mt-1 text-xs text-amber-800">
              {isZh ? '用途标签：' : 'Use flag: '}
              {clinicalUse}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{isZh ? 'AI 检测摘要' : 'AI Detection Summary'}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid md:grid-cols-4 gap-4">
            <div>
              <div className="text-sm text-gray-600">{isZh ? '癌症概率' : 'Cancer Probability'}</div>
              <div className="text-3xl font-bold text-red-600">
                {(detection.cancerProbability * 100).toFixed(1)}%
              </div>
            </div>
            <div>
              <div className="text-sm text-gray-600">{isZh ? '风险等级' : 'Risk Level'}</div>
              <div className={`inline-block px-3 py-1 rounded-full text-sm font-medium ${risk.color}`}>
                {risk.text}
              </div>
            </div>
            <div>
              <div className="text-sm text-gray-600">{isZh ? '发现项数量' : 'Findings Detected'}</div>
              <div className="text-3xl font-bold text-blue-600">{annotations.length}</div>
            </div>
            <div>
              <div className="text-sm text-gray-600">{isZh ? '模型版本' : 'Model Version'}</div>
              <div className="font-medium">{detection.modelVersion}</div>
            </div>
          </div>

          <div className="mt-6 grid md:grid-cols-2 gap-6">
            <div>
              <div className="text-sm text-gray-600 mb-2">{isZh ? '主要发现项' : 'Top Findings'}</div>
              <div className="flex flex-wrap gap-2">
                {topFindings.length > 0 ? (
                  topFindings.map((finding) => (
                    <span
                      key={finding}
                      className="px-3 py-1 rounded-full text-xs font-medium bg-red-100 text-red-700"
                    >
                      {finding}
                    </span>
                  ))
                ) : (
                  <span className="text-sm text-gray-500">{isZh ? '暂无排序发现项' : 'No ranked findings'}</span>
                )}
              </div>
            </div>

            <div>
              <div className="text-sm text-gray-600 mb-2">{isZh ? '标签评分' : 'Label Scores'}</div>
              <div className="space-y-2">
                {scoreEntries.length > 0 ? (
                  scoreEntries.map(([label, score]) => (
                    <div key={label}>
                      <div className="flex items-center justify-between text-xs mb-1">
                        <span>{label}</span>
                        <span className="font-medium">{(score * 100).toFixed(1)}%</span>
                      </div>
                      <div className="w-full bg-gray-200 rounded-full h-2">
                        <div
                          className="bg-blue-600 h-2 rounded-full"
                          style={{ width: `${Math.max(0, Math.min(100, score * 100))}%` }}
                        />
                      </div>
                    </div>
                  ))
                ) : (
                  <span className="text-sm text-gray-500">{isZh ? '暂无标签评分' : 'No label scores'}</span>
                )}
              </div>
            </div>
          </div>

          <div className="mt-6 grid md:grid-cols-3 gap-4">
            <div className="rounded border p-3">
              <div className="text-xs text-gray-600">{isZh ? '高敏阈值决策' : 'High-Sensitivity Decision'}</div>
              <div className={`mt-1 text-sm font-semibold ${decisionHighSensitivity ? 'text-red-600' : 'text-green-600'}`}>
                {decisionHighSensitivity
                  ? isZh
                    ? '阳性（建议优先复核）'
                    : 'Positive (prioritize review)'
                  : isZh
                  ? '阴性'
                  : 'Negative'}
              </div>
            </div>
            <div className="rounded border p-3">
              <div className="text-xs text-gray-600">{isZh ? '高特阈值决策' : 'High-Specificity Decision'}</div>
              <div className={`mt-1 text-sm font-semibold ${decisionHighSpecificity ? 'text-red-600' : 'text-green-600'}`}>
                {decisionHighSpecificity
                  ? isZh
                    ? '阳性（高置信告警）'
                    : 'Positive (high-confidence alert)'
                  : isZh
                  ? '阴性'
                  : 'Negative'}
              </div>
            </div>
            <div className="rounded border p-3">
              <div className="text-xs text-gray-600">{isZh ? '校准温度' : 'Calibration Temperature'}</div>
              <div className="mt-1 text-sm font-semibold text-gray-800">
                {calibrationTemperature !== null ? calibrationTemperature.toFixed(2) : 'N/A'}
              </div>
            </div>
          </div>

          {screeningSummary && (
            <div className="mt-6 rounded border p-4">
              <div className="text-sm font-semibold mb-2">{isZh ? '核心筛查摘要' : 'Core Screening Summary'}</div>
              <div className="grid md:grid-cols-4 gap-3 text-sm">
                <div>
                  <div className="text-gray-600">{isZh ? '肺炎风险' : 'Pneumonia Risk'}</div>
                  <div className="font-semibold">{((screeningSummary.pneumoniaScore ?? 0) * 100).toFixed(1)}%</div>
                </div>
                <div>
                  <div className="text-gray-600">{isZh ? '病灶风险' : 'Lesion Risk'}</div>
                  <div className="font-semibold">{((screeningSummary.lesionScore ?? 0) * 100).toFixed(1)}%</div>
                </div>
                <div>
                  <div className="text-gray-600">{isZh ? '白肺评分' : 'White Lung Score'}</div>
                  <div className="font-semibold">{((screeningSummary.whiteLungScore ?? 0) * 100).toFixed(1)}%</div>
                </div>
                <div>
                  <div className="text-gray-600">{isZh ? '综合风险' : 'Overall Risk'}</div>
                  <div className="font-semibold">{((screeningSummary.overallScore ?? 0) * 100).toFixed(1)}%</div>
                </div>
                <div>
                  <div className="text-gray-600">{isZh ? '分诊优先级' : 'Triage Priority'}</div>
                  <div className="font-semibold">{screeningSummary.triagePriority ?? 'N/A'}</div>
                </div>
              </div>
              {screeningSummary.infectionCoverage && (
                <div className="mt-4">
                  <div className="text-xs text-gray-600 mb-2">{isZh ? '感染覆盖风险谱' : 'Infection Coverage Spectrum'}</div>
                  <div className="grid md:grid-cols-2 gap-2">
                    {Object.entries(screeningSummary.infectionCoverage)
                      .sort((a, b) => b[1] - a[1])
                      .map(([label, score]) => (
                        <div key={label} className="rounded border px-2 py-1 text-xs flex items-center justify-between">
                          <span>{getInfectionCoverageLabel(label, isZh)}</span>
                          <span className="font-semibold">{(score * 100).toFixed(1)}%</span>
                        </div>
                      ))}
                  </div>
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      <ImageViewer imageUrl={detection.image.filePath} detections={annotations} editable={true} />

      <Card>
        <CardHeader>
          <CardTitle>{isZh ? '医生审阅' : 'Medical Review'}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-2">{isZh ? '审阅状态' : 'Review Status'}</label>
            <div className="flex gap-2">
              <Button
                variant={status === 'REVIEWED' ? 'default' : 'outline'}
                onClick={() => setStatus('REVIEWED')}
                className="flex-1"
              >
                {isZh ? '已审阅' : 'Reviewed'}
              </Button>
              <Button
                variant={status === 'CONFIRMED' ? 'default' : 'outline'}
                onClick={() => setStatus('CONFIRMED')}
                className="flex-1"
              >
                {isZh ? '已确认' : 'Confirmed'}
              </Button>
            </div>
          </div>

          <div>
            <label htmlFor="notes" className="block text-sm font-medium mb-2">
              {isZh ? '临床备注' : 'Clinical Notes'}
            </label>
            <textarea
              id="notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              className="w-full h-32 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder={isZh ? '输入临床观察与建议' : 'Enter clinical observations and recommendations'}
            />
          </div>

          <div className="flex gap-3 pt-4">
            <Button
              onClick={handleSubmit}
              disabled={reviewMutation.isPending || reportUpsertMutation.isPending || !notes.trim()}
              className="flex-1"
            >
              {reviewMutation.isPending || reportUpsertMutation.isPending ? (isZh ? '提交中...' : 'Submitting...') : isZh ? '提交审阅' : 'Submit Review'}
            </Button>
            <Button
              variant="outline"
              onClick={() => router.back()}
              disabled={reviewMutation.isPending || reportUpsertMutation.isPending}
            >
              {isZh ? '取消' : 'Cancel'}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
