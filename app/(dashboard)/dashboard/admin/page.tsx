'use client'

import { useEffect, useState } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ActivityChart, DetectionChart, UserGrowthChart, ModelPerformanceChart } from '@/components/charts/charts'
import { formatDate } from '@/lib/utils'
import { useI18n } from '@/components/i18n-provider'
import { useAuth } from '@/components/auth-provider'

type AdminOverview = {
  stats: {
    users: number
    doctors: number
    patients: number
    images: number
    detections: number
    pendingDetections: number
    reports: number
    finalizedReports: number
  }
  recentUsers: Array<{ id: string; name: string; email: string; role: string; createdAt: string }>
  recentActivity: Array<{ id: string; action: string; result: string; entityType: string; entityId: string; createdAt: string; actorUser?: { name?: string } | null }>
}

type OpsDashboard = {
  evidenceGate?: { pass?: boolean }
  latestEvidence?: { modelVersion?: string; createdAt?: string }
  openIncidents?: number
  p0p1Incidents?: number
}

type Readiness = {
  overallReady?: boolean
  db?: { ready?: boolean }
  ai?: { aiReachable?: boolean }
}

export default function AdminPage() {
  const { locale } = useI18n()
  const isZh = locale === 'zh'
  const { status, authFetch } = useAuth()
  const [data, setData] = useState<AdminOverview | null>(null)
  const [opsDashboard, setOpsDashboard] = useState<OpsDashboard | null>(null)
  const [readiness, setReadiness] = useState<Readiness | null>(null)
  const [incidents, setIncidents] = useState<Array<{ id: string; title: string; severity: string; status: string; source: string; openedAt: string }>>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (status !== 'authenticated') return
    let cancelled = false
    const load = async () => {
      setError('')
      try {
        const [overviewRes, dashboardRes, readinessRes, incidentsRes] = await Promise.all([
          authFetch('/api/v1/analytics/admin-overview'),
          authFetch('/api/v1/ops/dashboard'),
          authFetch('/api/v1/ops/readiness'),
          authFetch('/api/v1/ops/incidents?limit=8'),
        ])
        const overview = await overviewRes.json().catch(() => null)
        const dashboard = await dashboardRes.json().catch(() => null)
        const ready = await readinessRes.json().catch(() => null)
        const incidentList = await incidentsRes.json().catch(() => [])
        if (!overviewRes.ok) throw new Error(overview?.error || 'Failed to load analytics')
        if (cancelled) return
        setData(overview)
        setOpsDashboard(dashboard)
        setReadiness(ready)
        setIncidents(Array.isArray(incidentList) ? incidentList : [])
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : 'Failed to load analytics')
      } finally {
        if (!cancelled) setIsLoading(false)
      }
    }
    void load()
    const timer = setInterval(async () => {
      const readyRes = await authFetch('/api/v1/ops/readiness')
      const ready = await readyRes.json().catch(() => null)
      if (!cancelled) setReadiness(ready)
    }, 15000)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [status, authFetch])

  if (isLoading) return <div className="p-6 text-gray-500">{isZh ? '正在加载管理分析...' : 'Loading admin analytics...'}</div>
  if (error || !data) return <div className="p-6 text-red-600">{error || (isZh ? '加载分析失败' : 'Failed to load analytics')}</div>

  const { stats, recentUsers, recentActivity } = data
  const incidentList = incidents as Array<{ id: string; title: string; severity: string; status: string; source: string; openedAt: string }>

  return (
    <div className="space-y-6">
      <div><h1 className="text-3xl font-bold text-gray-900">{isZh ? '管理后台' : 'Admin Dashboard'}</h1></div>

      <div className="grid md:grid-cols-4 gap-6">
        <Card><CardHeader><CardTitle className="text-sm text-gray-600">{isZh ? '用户总数' : 'Total Users'}</CardTitle></CardHeader><CardContent><div className="text-3xl font-bold text-blue-600">{stats.users}</div><div className="mt-2 text-sm text-gray-500">{stats.doctors} {isZh ? '医生' : 'doctors'} • {stats.patients} {isZh ? '患者' : 'patients'}</div></CardContent></Card>
        <Card><CardHeader><CardTitle className="text-sm text-gray-600">{isZh ? '影像总数' : 'Total Images'}</CardTitle></CardHeader><CardContent><div className="text-3xl font-bold text-purple-600">{stats.images}</div></CardContent></Card>
        <Card><CardHeader><CardTitle className="text-sm text-gray-600">{isZh ? '检测数' : 'Detections'}</CardTitle></CardHeader><CardContent><div className="text-3xl font-bold text-green-600">{stats.detections}</div><div className="mt-2 text-sm text-gray-500">{stats.pendingDetections} {isZh ? '待审' : 'pending review'}</div></CardContent></Card>
        <Card><CardHeader><CardTitle className="text-sm text-gray-600">{isZh ? '报告数' : 'Reports'}</CardTitle></CardHeader><CardContent><div className="text-3xl font-bold text-orange-600">{stats.reports}</div><div className="mt-2 text-sm text-gray-500">{stats.finalizedReports} {isZh ? '已定稿' : 'finalized'}</div></CardContent></Card>
      </div>

      <div className="grid md:grid-cols-3 gap-6">
        <Card><CardHeader><CardTitle className="text-sm text-gray-600">{isZh ? '系统就绪状态' : 'System Readiness'}</CardTitle></CardHeader><CardContent><div className={`text-2xl font-bold ${readiness?.overallReady ? 'text-green-600' : 'text-red-600'}`}>{readiness?.overallReady ? (isZh ? '就绪' : 'READY') : (isZh ? '未就绪' : 'NOT READY')}</div><div className="mt-2 text-sm text-gray-500">DB: {readiness?.db?.ready ? 'OK' : 'DOWN'} • AI: {readiness?.ai?.aiReachable ? 'OK' : 'DOWN'}</div></CardContent></Card>
        <Card><CardHeader><CardTitle className="text-sm text-gray-600">{isZh ? '临床证据门禁' : 'Clinical Evidence Gate'}</CardTitle></CardHeader><CardContent><div className={`text-2xl font-bold ${opsDashboard?.evidenceGate?.pass ? 'text-green-600' : 'text-amber-600'}`}>{opsDashboard?.evidenceGate?.pass ? (isZh ? '通过' : 'PASS') : (isZh ? '未通过' : 'NOT PASS')}</div><div className="mt-2 text-sm text-gray-500">{opsDashboard?.latestEvidence?.createdAt ? `${opsDashboard.latestEvidence.modelVersion ?? ''} • ${formatDate(opsDashboard.latestEvidence.createdAt)}` : isZh ? '暂无证据记录' : 'No evidence record'}</div></CardContent></Card>
        <Card><CardHeader><CardTitle className="text-sm text-gray-600">{isZh ? '高优先级事件' : 'High Priority Incidents'}</CardTitle></CardHeader><CardContent><div className="text-3xl font-bold text-red-600">{opsDashboard?.p0p1Incidents ?? 0}</div><div className="mt-2 text-sm text-gray-500">{isZh ? '未关闭总事件' : 'Open incidents'}: {opsDashboard?.openIncidents ?? 0}</div></CardContent></Card>
      </div>

      <div className="grid md:grid-cols-2 gap-6">
        <Card><CardHeader><CardTitle>{isZh ? '用户活动（示例图）' : 'User Activity (Demo Chart)'}</CardTitle></CardHeader><CardContent><ActivityChart /></CardContent></Card>
        <Card><CardHeader><CardTitle>{isZh ? '检测分布（示例图）' : 'Detection Distribution (Demo Chart)'}</CardTitle></CardHeader><CardContent><DetectionChart /></CardContent></Card>
      </div>

      <Card><CardHeader><CardTitle>{isZh ? '用户增长（示例图）' : 'User Growth (Demo Chart)'}</CardTitle></CardHeader><CardContent><UserGrowthChart /></CardContent></Card>

      <Card><CardHeader><CardTitle>{isZh ? '最近用户' : 'Recent Users'}</CardTitle></CardHeader><CardContent><div className="space-y-4">{recentUsers.map((user) => (<div key={user.id} className="flex items-center justify-between p-3 bg-gray-50 rounded"><div><div className="font-medium">{user.name}</div><div className="text-sm text-gray-600">{user.email}</div></div><div className="text-right"><div className={`text-xs px-2 py-1 rounded-full ${user.role === 'DOCTOR' ? 'bg-blue-100 text-blue-700' : 'bg-gray-100 text-gray-700'}`}>{user.role}</div><div className="text-xs text-gray-500 mt-1">{formatDate(user.createdAt)}</div></div></div>))}</div></CardContent></Card>

      <Card><CardHeader><CardTitle>{isZh ? '最近审计活动' : 'Recent Audit Activity'}</CardTitle></CardHeader><CardContent><div className="space-y-3">{recentActivity.map((activity) => (<div key={activity.id} className="p-3 bg-gray-50 rounded"><div className="text-sm font-medium">{activity.action} [{activity.result}]</div><div className="text-xs text-gray-600 mt-1">{activity.entityType}:{activity.entityId}</div><div className="text-xs text-gray-500 mt-1">{activity.actorUser?.name ?? (isZh ? '系统' : 'system')} • {formatDate(activity.createdAt)}</div></div>))}</div></CardContent></Card>

      <Card><CardHeader><CardTitle>{isZh ? '运维事件' : 'Operational Incidents'}</CardTitle></CardHeader><CardContent><div className="space-y-3">{incidentList.map((incident) => (<div key={incident.id} className="p-3 bg-gray-50 rounded"><div className="flex items-center justify-between gap-3"><div className="text-sm font-medium">{incident.title}</div><div className="text-xs text-gray-600">{incident.severity} • {incident.status}</div></div><div className="text-xs text-gray-600 mt-1">{isZh ? '来源' : 'Source'}: {incident.source}</div><div className="text-xs text-gray-500 mt-1">{formatDate(incident.openedAt)}</div></div>))}</div></CardContent></Card>
      <Card><CardHeader><CardTitle>{isZh ? 'AI模型性能（示例图）' : 'AI Model Performance (Demo Chart)'}</CardTitle></CardHeader><CardContent><ModelPerformanceChart /></CardContent></Card>
    </div>
  )
}
