'use client'

import { useEffect, useMemo, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import ImageViewer from '@/components/image-viewer/image-viewer'
import { useI18n } from '@/components/i18n-provider'
import { getInfectionCoverageLabel } from '@/lib/screening'
import { formatDate } from '@/lib/utils'
import { useAuth } from '@/components/auth-provider'

type DetectionDetail = {
  id: string
  cancerProbability: number
  findings: unknown
  modelVersion: string
  image: {
    filePath: string
    createdAt: string
    patient: {
      id: string
      dateOfBirth: string
      gender: string
      user: { name: string; email: string }
    }
  }
}

type AnnotationRect = { id: string; x: number; y: number; width: number; height: number; label: string; confidence?: number; color: string }

function calculateAge(dob: Date | string) {
  const birth = new Date(dob)
  const today = new Date()
  let age = today.getFullYear() - birth.getFullYear()
  const monthDiff = today.getMonth() - birth.getMonth()
  if (monthDiff < 0 || (monthDiff === 0 && today.getDate() < birth.getDate())) age--
  return age
}

function extractAnnotations(findings: unknown): AnnotationRect[] {
  if (!findings || typeof findings !== 'object') return []
  const regions = (findings as { regions?: unknown }).regions
  if (!Array.isArray(regions)) return []
  return regions
    .filter((region): region is Record<string, unknown> => !!region && typeof region === 'object')
    .filter((r) => typeof r.x === 'number' && typeof r.y === 'number' && typeof r.width === 'number' && typeof r.height === 'number')
    .map((r, idx) => ({ id: `ann-${idx}`, x: r.x as number, y: r.y as number, width: r.width as number, height: r.height as number, label: typeof r.label === 'string' ? r.label : 'Finding', confidence: typeof r.confidence === 'number' ? r.confidence : undefined, color: '#ef4444' }))
}

export default function ReviewPage() {
  const { locale } = useI18n()
  const isZh = locale === 'zh'
  const router = useRouter()
  const params = useParams<{ id: string }>()
  const detectionId = params.id
  const { status: authStatus, authFetch } = useAuth()
  const [detection, setDetection] = useState<DetectionDetail | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState('')
  const [status, setStatus] = useState<'REVIEWED' | 'CONFIRMED'>('REVIEWED')
  const [notes, setNotes] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (authStatus !== 'authenticated' || !detectionId) return
    let cancelled = false
    const load = async () => {
      setIsLoading(true)
      try {
        const response = await authFetch(`/api/v1/detections/${detectionId}`)
        const data = await response.json()
        if (!response.ok) throw new Error(typeof data?.error === 'string' ? data.error : 'Load failed')
        if (!cancelled) setDetection(data as DetectionDetail)
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : 'Load failed')
      } finally {
        if (!cancelled) setIsLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [authStatus, detectionId, authFetch])

  const annotations = useMemo(() => extractAnnotations(detection?.findings), [detection?.findings])
  const screeningSummary = useMemo(() => {
    if (!detection?.findings || typeof detection.findings !== 'object') return null
    const summary = (detection.findings as { screeningSummary?: unknown }).screeningSummary
    return summary && typeof summary === 'object' ? (summary as Record<string, unknown>) : null
  }, [detection?.findings])
  const infectionCoverage = useMemo(() => {
    if (!screeningSummary) return null
    const value = screeningSummary.infectionCoverage
    if (!value || typeof value !== 'object') return null
    return value as Record<string, number>
  }, [screeningSummary])

  const handleSubmit = async () => {
    if (!detectionId || !notes.trim() || !detection) return
    setIsSubmitting(true)
    try {
      const reviewRes = await authFetch(`/api/v1/detections/${detectionId}/review`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status, reviewNotes: notes }),
      })
      const reviewData = await reviewRes.json().catch(() => ({}))
      if (!reviewRes.ok) throw new Error(typeof reviewData?.error === 'string' ? reviewData.error : 'Review failed')

      const reportRes = await authFetch('/api/v1/reports', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          detectionId: detection.id,
          patientId: detection.image.patient.id,
          status: status === 'CONFIRMED' ? 'FINALIZED' : 'DRAFT',
          content: {
            diagnosis: status === 'CONFIRMED' ? (isZh ? '医生已确认：存在需进一步临床评估的可疑病灶' : 'Confirmed by doctor: suspicious lesion requires further clinical evaluation') : (isZh ? '医生已复核：建议继续观察并结合临床信息判断' : 'Reviewed by doctor: continue observation and correlate with clinical context'),
            doctorNotes: notes,
            aiCancerProbability: detection.cancerProbability,
            screeningSummary,
          },
        }),
      })
      const reportData = await reportRes.json().catch(() => ({}))
      if (!reportRes.ok) throw new Error(typeof reportData?.error === 'string' ? reportData.error : 'Report save failed')
      router.push('/dashboard/doctor/queue')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Submit failed')
    } finally {
      setIsSubmitting(false)
    }
  }

  if (isLoading) return <div className="p-6 text-gray-500">{isZh ? '正在加载检测结果...' : 'Loading detection...'}</div>
  if (error || !detection) return <div className="p-6 text-red-600">{error || (isZh ? '未找到检测记录' : 'Detection not found')}</div>

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between"><div><h1 className="text-3xl font-bold text-gray-900">{isZh ? '检测审阅' : 'Review Detection'}</h1><p className="text-gray-600 mt-2">{isZh ? '患者：' : 'Patient: '}{detection.image.patient.user.name}</p></div><Button variant="outline" onClick={() => router.back()}>{isZh ? '返回队列' : 'Back to Queue'}</Button></div>
      <Card><CardHeader><CardTitle>{isZh ? '患者信息' : 'Patient Information'}</CardTitle></CardHeader><CardContent><div className="grid md:grid-cols-4 gap-4"><div><div className="text-sm text-gray-600">{isZh ? '姓名' : 'Name'}</div><div className="font-medium">{detection.image.patient.user.name}</div></div><div><div className="text-sm text-gray-600">{isZh ? '年龄 / 性别' : 'Age / Gender'}</div><div className="font-medium">{calculateAge(detection.image.patient.dateOfBirth)} / {detection.image.patient.gender}</div></div><div><div className="text-sm text-gray-600">{isZh ? '邮箱' : 'Email'}</div><div className="font-medium">{detection.image.patient.user.email}</div></div><div><div className="text-sm text-gray-600">{isZh ? '上传日期' : 'Upload Date'}</div><div className="font-medium">{formatDate(detection.image.createdAt)}</div></div></div></CardContent></Card>
      <ImageViewer imageUrl={detection.image.filePath} detections={annotations} editable={true} />
      {infectionCoverage && (
        <Card><CardHeader><CardTitle>{isZh ? '感染覆盖风险谱' : 'Infection Coverage Spectrum'}</CardTitle></CardHeader><CardContent><div className="grid md:grid-cols-2 gap-2">{Object.entries(infectionCoverage).sort((a, b) => b[1] - a[1]).map(([label, score]) => (<div key={label} className="rounded border px-2 py-1 text-xs flex items-center justify-between"><span>{getInfectionCoverageLabel(label, isZh)}</span><span className="font-semibold">{(score * 100).toFixed(1)}%</span></div>))}</div></CardContent></Card>
      )}
      <Card><CardHeader><CardTitle>{isZh ? '医生审阅' : 'Medical Review'}</CardTitle></CardHeader><CardContent className="space-y-4"><div><label className="block text-sm font-medium mb-2">{isZh ? '审阅状态' : 'Review Status'}</label><div className="flex gap-2"><Button variant={status === 'REVIEWED' ? 'default' : 'outline'} onClick={() => setStatus('REVIEWED')} className="flex-1">{isZh ? '已审阅' : 'Reviewed'}</Button><Button variant={status === 'CONFIRMED' ? 'default' : 'outline'} onClick={() => setStatus('CONFIRMED')} className="flex-1">{isZh ? '已确认' : 'Confirmed'}</Button></div></div><div><label htmlFor="notes" className="block text-sm font-medium mb-2">{isZh ? '临床备注' : 'Clinical Notes'}</label><textarea id="notes" value={notes} onChange={(e) => setNotes(e.target.value)} className="w-full h-32 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500" placeholder={isZh ? '输入临床观察与建议' : 'Enter clinical observations and recommendations'} /></div><div className="flex gap-3 pt-4"><Button onClick={handleSubmit} disabled={isSubmitting || !notes.trim()} className="flex-1">{isSubmitting ? (isZh ? '提交中...' : 'Submitting...') : isZh ? '提交审阅' : 'Submit Review'}</Button><Button variant="outline" onClick={() => router.back()} disabled={isSubmitting}>{isZh ? '取消' : 'Cancel'}</Button></div></CardContent></Card>
    </div>
  )
}
