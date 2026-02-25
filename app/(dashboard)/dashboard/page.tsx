'use client'

import { useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import Link from 'next/link'
import { formatDate } from '@/lib/utils'
import { useI18n } from '@/components/i18n-provider'
import { useAuth } from '@/components/auth-provider'

type ImageItem = { id: string; status: string }
type DetectionItem = { id: string; status: string; cancerProbability: number; updatedAt: string }
type ReportItem = { id: string }
type AuditItem = { id: string; action: string; result: string; entityType: string; entityId: string; createdAt: string }

export default function DashboardPage() {
  const { locale } = useI18n()
  const isZh = locale === 'zh'
  const router = useRouter()
  const { user, status, authFetch } = useAuth()
  const [isLoading, setIsLoading] = useState(true)
  const [images, setImages] = useState<ImageItem[]>([])
  const [detections, setDetections] = useState<DetectionItem[]>([])
  const [reports, setReports] = useState<ReportItem[]>([])
  const [activity, setActivity] = useState<AuditItem[]>([])

  useEffect(() => {
    if (status === 'unauthenticated') {
      router.push('/login')
      return
    }
    if (status !== 'authenticated' || !user) return
    let cancelled = false
    const load = async () => {
      setIsLoading(true)
      try {
        const [imagesRes, detectionsRes, reportsRes, auditsRes] = await Promise.all([
          authFetch('/api/v1/images?limit=100'),
          authFetch('/api/v1/detections?limit=100&orderByPriority=false'),
          authFetch('/api/v1/reports?limit=100'),
          user.role === 'DOCTOR' || user.role === 'ADMIN'
            ? authFetch('/api/v1/audits?limit=10')
            : Promise.resolve(new Response(JSON.stringify({ logs: [] }), { status: 200 })),
        ])
        const imagesData = await imagesRes.json().catch(() => ({ images: [] }))
        const detectionsData = await detectionsRes.json().catch(() => ({ detections: [] }))
        const reportsData = await reportsRes.json().catch(() => ({ reports: [] }))
        const auditsData = await auditsRes.json().catch(() => ({ logs: [] }))
        if (cancelled) return
        setImages(Array.isArray(imagesData.images) ? imagesData.images : [])
        setDetections(Array.isArray(detectionsData.detections) ? detectionsData.detections : [])
        setReports(Array.isArray(reportsData.reports) ? reportsData.reports : [])
        setActivity(Array.isArray(auditsData.logs) ? auditsData.logs : [])
      } finally {
        if (!cancelled) setIsLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [status, user, authFetch, router])

  if (status === 'loading' || isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-gray-600">{isZh ? '加载中...' : 'Loading...'}</div>
      </div>
    )
  }

  if (!user) return null

  const userRole = user.role
  const pendingImages = images.filter((image) => image.status === 'PENDING' || image.status === 'PROCESSING').length
  const pendingDetections = detections.filter((detection) => detection.status === 'PENDING').length
  const reviewedToday = detections.filter((detection) => {
    const date = new Date(detection.updatedAt)
    const now = new Date()
    return (
      detection.status !== 'PENDING' &&
      date.getFullYear() === now.getFullYear() &&
      date.getMonth() === now.getMonth() &&
      date.getDate() === now.getDate()
    )
  }).length

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-gray-900">
          {isZh ? '欢迎，' : 'Welcome, '}
          {user.name}
        </h1>
      </div>

      {userRole === 'PATIENT' && (
        <div className="grid md:grid-cols-3 gap-6">
          <Card><CardHeader><CardTitle className="text-lg">{isZh ? '我的影像' : 'My Images'}</CardTitle></CardHeader><CardContent><div className="text-4xl font-bold text-blue-600 mb-2">{images.length}</div><Link href="/dashboard/images"><Button variant="outline" size="sm" className="w-full">{isZh ? '查看全部' : 'View All'}</Button></Link></CardContent></Card>
          <Card><CardHeader><CardTitle className="text-lg">{isZh ? '待处理' : 'Pending Reviews'}</CardTitle></CardHeader><CardContent><div className="text-4xl font-bold text-yellow-600 mb-2">{pendingImages}</div><Link href="/dashboard/upload"><Button size="sm" className="w-full">{isZh ? '上传新影像' : 'Upload New'}</Button></Link></CardContent></Card>
          <Card><CardHeader><CardTitle className="text-lg">{isZh ? '报告' : 'Reports'}</CardTitle></CardHeader><CardContent><div className="text-4xl font-bold text-green-600 mb-2">{reports.length}</div><Link href="/dashboard/reports"><Button variant="outline" size="sm" className="w-full">{isZh ? '查看报告' : 'View Reports'}</Button></Link></CardContent></Card>
        </div>
      )}

      {userRole === 'DOCTOR' && (
        <div className="grid md:grid-cols-4 gap-6">
          <Card><CardHeader><CardTitle className="text-lg">{isZh ? '待审队列' : 'Review Queue'}</CardTitle></CardHeader><CardContent><div className="text-4xl font-bold text-orange-600 mb-2">{pendingDetections}</div><Link href="/dashboard/doctor/queue"><Button size="sm" className="w-full">{isZh ? '开始审阅' : 'Start Review'}</Button></Link></CardContent></Card>
          <Card><CardHeader><CardTitle className="text-lg">{isZh ? '今日已审阅' : 'Today Reviews'}</CardTitle></CardHeader><CardContent><div className="text-4xl font-bold text-blue-600 mb-2">{reviewedToday}</div></CardContent></Card>
          <Card><CardHeader><CardTitle className="text-lg">{isZh ? '报告' : 'Reports'}</CardTitle></CardHeader><CardContent><div className="text-4xl font-bold text-purple-600 mb-2">{reports.length}</div><Link href="/dashboard/reports"><Button variant="outline" size="sm" className="w-full">{isZh ? '查看报告' : 'View Reports'}</Button></Link></CardContent></Card>
          <Card><CardHeader><CardTitle className="text-lg">{isZh ? '高风险待审' : 'High Risk Pending'}</CardTitle></CardHeader><CardContent><div className="text-4xl font-bold text-red-600 mb-2">{detections.filter((item) => item.status === 'PENDING' && item.cancerProbability >= 0.7).length}</div></CardContent></Card>
        </div>
      )}

      <Card>
        <CardHeader><CardTitle>{isZh ? '最近活动' : 'Recent Activity'}</CardTitle></CardHeader>
        <CardContent>
          {activity.length === 0 ? (
            <div className="text-center py-8 text-gray-500">{isZh ? '暂无最近活动' : 'No recent activity'}</div>
          ) : (
            <div className="space-y-3">
              {activity.map((item) => (
                <div key={item.id} className="p-3 bg-gray-50 rounded">
                  <div className="text-sm font-medium">{item.action} [{item.result}]</div>
                  <div className="text-xs text-gray-600 mt-1">{item.entityType}:{item.entityId}</div>
                  <div className="text-xs text-gray-500 mt-1">{formatDate(item.createdAt)}</div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
