'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import Link from 'next/link'
import { formatTime } from '@/lib/utils'
import { useI18n } from '@/components/i18n-provider'
import { useAuth } from '@/components/auth-provider'

type Detection = {
  id: string
  modelVersion: string
  status: string
  cancerProbability: number
  findings: unknown
  image: {
    originalName: string
    createdAt: string
    patient: { user: { name: string } }
  }
}

export default function DoctorQueuePage() {
  const { locale } = useI18n()
  const isZh = locale === 'zh'
  const { status, authFetch } = useAuth()
  const [filter, setFilter] = useState<'ALL' | 'HIGH' | 'MEDIUM' | 'LOW'>('ALL')
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState('')
  const [detections, setDetections] = useState<Detection[]>([])

  useEffect(() => {
    if (status !== 'authenticated') return
    let cancelled = false
    const load = async () => {
      setIsLoading(true)
      setError('')
      try {
        const response = await authFetch('/api/v1/detections?status=PENDING&orderByPriority=true&limit=100')
        const data = await response.json()
        if (!response.ok) throw new Error(typeof data?.error === 'string' ? data.error : 'Load failed')
        if (!cancelled) setDetections(Array.isArray(data.detections) ? data.detections : [])
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : 'Load failed')
      } finally {
        if (!cancelled) setIsLoading(false)
      }
    }
    void load()
    const timer = setInterval(load, 10000)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [status, authFetch])

  const getScreeningSummary = (findings: unknown) => {
    if (!findings || typeof findings !== 'object') return null
    const summary = (findings as { screeningSummary?: unknown }).screeningSummary
    if (!summary || typeof summary !== 'object') return null
    return summary as {
      triagePriority?: 'CRITICAL' | 'HIGH' | 'ROUTINE'
      pneumoniaScore?: number
      lesionScore?: number
      whiteLungScore?: number
      suspectedConditions?: string[]
    }
  }

  const getRiskLevel = (probability: number) => {
    if (probability < 0.3) return { text: isZh ? '低风险' : 'Low Risk', color: 'text-green-600 bg-green-100', priority: isZh ? '低' : 'Low' }
    if (probability < 0.7) return { text: isZh ? '中风险' : 'Medium Risk', color: 'text-yellow-600 bg-yellow-100', priority: isZh ? '中' : 'Medium' }
    return { text: isZh ? '高风险' : 'High Risk', color: 'text-red-600 bg-red-100', priority: isZh ? '高' : 'High' }
  }

  const getPriorityBucket = useCallback((detection: Detection) => {
    const summary = getScreeningSummary(detection.findings)
    const triage = summary?.triagePriority
    if (triage === 'CRITICAL') return 'HIGH'
    if (triage === 'HIGH') return 'MEDIUM'
    if (triage === 'ROUTINE') return 'LOW'
    if (detection.cancerProbability >= 0.7) return 'HIGH'
    if (detection.cancerProbability >= 0.3) return 'MEDIUM'
    return 'LOW'
  }, [])

  const stats = useMemo(() => {
    const high = detections.filter((d) => getPriorityBucket(d) === 'HIGH').length
    const medium = detections.filter((d) => getPriorityBucket(d) === 'MEDIUM').length
    const low = detections.filter((d) => getPriorityBucket(d) === 'LOW').length
    return { high, medium, low, all: detections.length }
  }, [detections, getPriorityBucket])

  const visibleDetections = useMemo(() => {
    if (filter === 'HIGH') return detections.filter((d) => getPriorityBucket(d) === 'HIGH')
    if (filter === 'MEDIUM') return detections.filter((d) => getPriorityBucket(d) === 'MEDIUM')
    if (filter === 'LOW') return detections.filter((d) => getPriorityBucket(d) === 'LOW')
    return detections
  }, [detections, filter, getPriorityBucket])

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-gray-900">{isZh ? '审阅队列' : 'Review Queue'}</h1>
      </div>

      <Card><CardContent className="pt-6"><div className="flex gap-2">
        <Button variant={filter === 'ALL' ? 'default' : 'outline'} size="sm" onClick={() => setFilter('ALL')}>{isZh ? '全部' : 'All'} ({stats.all})</Button>
        <Button variant={filter === 'HIGH' ? 'default' : 'outline'} size="sm" onClick={() => setFilter('HIGH')}>{isZh ? '高优先级' : 'High Priority'} ({stats.high})</Button>
        <Button variant={filter === 'MEDIUM' ? 'default' : 'outline'} size="sm" onClick={() => setFilter('MEDIUM')}>{isZh ? '中优先级' : 'Medium Priority'} ({stats.medium})</Button>
        <Button variant={filter === 'LOW' ? 'default' : 'outline'} size="sm" onClick={() => setFilter('LOW')}>{isZh ? '低优先级' : 'Low Priority'} ({stats.low})</Button>
      </div></CardContent></Card>

      {isLoading && <Card><CardContent className="py-8 text-center text-gray-500">{isZh ? '正在加载队列...' : 'Loading queue...'}</CardContent></Card>}
      {error && <Card><CardContent className="py-8 text-center text-red-600">{error}</CardContent></Card>}

      {!isLoading && !error && visibleDetections.length === 0 ? (
        <Card><CardContent className="py-12"><div className="text-center"><div className="text-6xl mb-4">✅</div><p className="text-gray-500">{isZh ? '暂无待审病例' : 'No cases pending review'}</p></div></CardContent></Card>
      ) : (
        <div className="space-y-4">
          {visibleDetections.map((detection) => {
            const risk = getRiskLevel(detection.cancerProbability)
            return (
              <Card key={detection.id} className="hover:shadow-md transition-shadow">
                <CardContent className="pt-6">
                  <div className="flex items-center gap-6">
                    <div className="flex flex-col items-center"><div className={`w-12 h-12 rounded-full ${risk.color} flex items-center justify-center font-bold text-lg`}>{risk.priority[0]}</div></div>
                    <div className="flex-1">
                      <div className="flex items-start justify-between">
                        <div>
                          <h3 className="font-semibold text-lg">{detection.image.patient.user.name ?? (isZh ? '未知患者' : 'Unknown Patient')}</h3>
                          <p className="text-sm text-gray-600">{detection.image.originalName}</p>
                          <p className="text-xs text-gray-500 mt-1">{isZh ? '上传时间：' : 'Uploaded: '}{formatTime(detection.image.createdAt)}</p>
                        </div>
                        <div className="text-right">
                          <div className={`text-2xl font-bold ${risk.color.split(' ')[0]}`}>{(detection.cancerProbability * 100).toFixed(1)}%</div>
                          <div className={`text-xs px-2 py-1 rounded-full mt-1 ${risk.color}`}>{risk.text}</div>
                        </div>
                      </div>
                      <div className="mt-3 pt-3 border-t flex items-center justify-between">
                        <div className="text-xs text-gray-500">{isZh ? '模型：' : 'Model: '}{detection.modelVersion} • {isZh ? '状态：' : 'Status: '}{detection.status}</div>
                        <div className="flex gap-2">
                          <Link href={`/dashboard/doctor/review/${detection.id}`}><Button>{isZh ? '开始审阅' : 'Start Review'}</Button></Link>
                          <Link href={`/dashboard/doctor/review/${detection.id}`}><Button variant="outline">{isZh ? '查看详情' : 'View Details'}</Button></Link>
                        </div>
                      </div>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}
