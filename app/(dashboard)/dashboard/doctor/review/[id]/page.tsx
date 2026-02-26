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
  const { locale, t } = useI18n()
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
            diagnosis: status === 'CONFIRMED' ? t('review.confirmedDiagnosis') : t('review.reviewedDiagnosis'),
            doctorNotes: notes,
            aiCancerProbability: detection.cancerProbability,
            aiLesionProbability: detection.cancerProbability,
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

  if (isLoading) return <div className="p-6 text-gray-500">{t('review.loadingDetection')}</div>
  if (error || !detection) return <div className="p-6 text-red-600">{error || t('review.notFound')}</div>

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between"><div><h1 className="text-3xl font-bold text-gray-900">{t('review.title')}</h1><p className="text-gray-600 mt-2">{t('review.patientPrefix')}{detection.image.patient.user.name}</p></div><Button variant="outline" onClick={() => router.back()}>{t('review.backToQueue')}</Button></div>
      <Card><CardHeader><CardTitle>{t('review.patientInfo')}</CardTitle></CardHeader><CardContent><div className="grid md:grid-cols-4 gap-4"><div><div className="text-sm text-gray-600">{t('review.name')}</div><div className="font-medium">{detection.image.patient.user.name}</div></div><div><div className="text-sm text-gray-600">{t('review.ageGender')}</div><div className="font-medium">{calculateAge(detection.image.patient.dateOfBirth)} / {detection.image.patient.gender}</div></div><div><div className="text-sm text-gray-600">{t('review.email')}</div><div className="font-medium">{detection.image.patient.user.email}</div></div><div><div className="text-sm text-gray-600">{t('review.uploadDate')}</div><div className="font-medium">{formatDate(detection.image.createdAt)}</div></div></div></CardContent></Card>
      <ImageViewer imageUrl={detection.image.filePath} detections={annotations} editable={true} />
      {infectionCoverage && (
        <Card><CardHeader><CardTitle>{t('review.infectionCoverage')}</CardTitle></CardHeader><CardContent><div className="grid md:grid-cols-2 gap-2">{Object.entries(infectionCoverage).sort((a, b) => b[1] - a[1]).map(([label, score]) => (<div key={label} className="rounded border px-2 py-1 text-xs flex items-center justify-between"><span>{getInfectionCoverageLabel(label, locale)}</span><span className="font-semibold">{(score * 100).toFixed(1)}%</span></div>))}</div></CardContent></Card>
      )}
      <Card><CardHeader><CardTitle>{t('review.medicalReview')}</CardTitle></CardHeader><CardContent className="space-y-4"><div><label className="block text-sm font-medium mb-2">{t('review.statusLabel')}</label><div className="flex gap-2"><Button variant={status === 'REVIEWED' ? 'default' : 'outline'} onClick={() => setStatus('REVIEWED')} className="flex-1">{t('review.statusReviewed')}</Button><Button variant={status === 'CONFIRMED' ? 'default' : 'outline'} onClick={() => setStatus('CONFIRMED')} className="flex-1">{t('review.statusConfirmed')}</Button></div></div><div><label htmlFor="notes" className="block text-sm font-medium mb-2">{t('review.clinicalNotes')}</label><textarea id="notes" value={notes} onChange={(e) => setNotes(e.target.value)} className="w-full h-32 px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500" placeholder={t('review.notesPlaceholder')} /></div><div className="flex gap-3 pt-4"><Button onClick={handleSubmit} disabled={isSubmitting || !notes.trim()} className="flex-1">{isSubmitting ? t('review.submitting') : t('review.submitReview')}</Button><Button variant="outline" onClick={() => router.back()} disabled={isSubmitting}>{t('common.cancel')}</Button></div></CardContent></Card>
    </div>
  )
}
