'use client'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ActivityChart, DetectionChart, UserGrowthChart, ModelPerformanceChart } from '@/components/charts/charts'
import { api } from '@/lib/trpc'
import { formatDate } from '@/lib/utils'
import { useI18n } from '@/components/i18n-provider'

export default function AdminPage() {
  const { locale } = useI18n()
  const isZh = locale === 'zh'
  const { data, isLoading, error } = api.analytics.adminOverview.useQuery()

  if (isLoading) {
    return <div className="p-6 text-gray-500">{isZh ? '正在加载管理分析...' : 'Loading admin analytics...'}</div>
  }

  if (error || !data) {
    return <div className="p-6 text-red-600">{error?.message ?? (isZh ? '加载分析失败' : 'Failed to load analytics')}</div>
  }

  const { stats, recentUsers, recentActivity } = data

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-gray-900">{isZh ? '管理后台' : 'Admin Dashboard'}</h1>
        <p className="text-gray-600 mt-2">{isZh ? '系统总览与管理' : 'System overview and management'}</p>
      </div>

      <div className="grid md:grid-cols-4 gap-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm text-gray-600">{isZh ? '用户总数' : 'Total Users'}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-blue-600">{stats.users}</div>
            <div className="mt-2 text-sm text-gray-500">
              {stats.doctors} {isZh ? '医生' : 'doctors'} • {stats.patients} {isZh ? '患者' : 'patients'}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm text-gray-600">{isZh ? '影像总数' : 'Total Images'}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-purple-600">{stats.images}</div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm text-gray-600">{isZh ? '检测数' : 'Detections'}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-green-600">{stats.detections}</div>
            <div className="mt-2 text-sm text-gray-500">{stats.pendingDetections} {isZh ? '待审' : 'pending review'}</div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm text-gray-600">{isZh ? '报告数' : 'Reports'}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-orange-600">{stats.reports}</div>
            <div className="mt-2 text-sm text-gray-500">{stats.finalizedReports} {isZh ? '已定稿' : 'finalized'}</div>
          </CardContent>
        </Card>
      </div>

      <div className="grid md:grid-cols-2 gap-6">
        <Card>
          <CardHeader>
            <CardTitle>{isZh ? '用户活动（示例图）' : 'User Activity (Demo Chart)'}</CardTitle>
          </CardHeader>
          <CardContent>
            <ActivityChart />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{isZh ? '检测分布（示例图）' : 'Detection Distribution (Demo Chart)'}</CardTitle>
          </CardHeader>
          <CardContent>
            <DetectionChart />
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{isZh ? '用户增长（示例图）' : 'User Growth (Demo Chart)'}</CardTitle>
        </CardHeader>
        <CardContent>
          <UserGrowthChart />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{isZh ? '最近用户' : 'Recent Users'}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {recentUsers.map((user) => (
              <div key={user.id} className="flex items-center justify-between p-3 bg-gray-50 rounded">
                <div>
                  <div className="font-medium">{user.name}</div>
                  <div className="text-sm text-gray-600">{user.email}</div>
                </div>
                <div className="text-right">
                  <div
                    className={`text-xs px-2 py-1 rounded-full ${
                      user.role === 'DOCTOR' ? 'bg-blue-100 text-blue-700' : 'bg-gray-100 text-gray-700'
                    }`}
                  >
                    {user.role}
                  </div>
                  <div className="text-xs text-gray-500 mt-1">{formatDate(user.createdAt)}</div>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{isZh ? '最近审计活动' : 'Recent Audit Activity'}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {recentActivity.length === 0 && (
              <div className="text-sm text-gray-500">{isZh ? '暂无活动' : 'No activity yet'}</div>
            )}
            {recentActivity.map((activity) => (
              <div key={activity.id} className="p-3 bg-gray-50 rounded">
                <div className="text-sm font-medium">
                  {activity.action} [{activity.result}]
                </div>
                <div className="text-xs text-gray-600 mt-1">
                  {isZh ? '实体' : 'Entity'}: {activity.entityType}:{activity.entityId}
                </div>
                <div className="text-xs text-gray-500 mt-1">
                  {isZh ? '操作人' : 'Actor'}: {activity.actorUser?.name ?? (isZh ? '系统' : 'system')} • {formatDate(activity.createdAt)}
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{isZh ? 'AI模型性能（示例图）' : 'AI Model Performance (Demo Chart)'}</CardTitle>
        </CardHeader>
        <CardContent>
          <ModelPerformanceChart />
        </CardContent>
      </Card>
    </div>
  )
}
