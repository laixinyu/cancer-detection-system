'use client'

import { useSession } from 'next-auth/react'
import { useRouter } from 'next/navigation'
import { useEffect } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import Link from 'next/link'
import { api } from '@/lib/trpc'
import { formatDate } from '@/lib/utils'
import { useI18n } from '@/components/i18n-provider'

export default function DashboardPage() {
  const { locale } = useI18n()
  const isZh = locale === 'zh'
  const { data: session, status } = useSession()
  const router = useRouter()

  const imageQuery = api.image.list.useQuery({ limit: 100 }, { enabled: status === 'authenticated' })
  const detectionQuery = api.detection.list.useQuery(
    { limit: 100, orderByPriority: false },
    { enabled: status === 'authenticated' }
  )
  const reportQuery = api.report.list.useQuery({ limit: 100 }, { enabled: status === 'authenticated' })
  const auditQuery = api.audit.list.useQuery(
    { limit: 10 },
    {
      enabled:
        status === 'authenticated' &&
        (session?.user.role === 'DOCTOR' || session?.user.role === 'ADMIN'),
    }
  )

  useEffect(() => {
    if (status === 'unauthenticated') {
      router.push('/login')
    }
  }, [status, router])

  if (status === 'loading') {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-gray-600">{isZh ? '加载中...' : 'Loading...'}</div>
      </div>
    )
  }

  if (!session) {
    return null
  }

  const userRole = session.user.role
  const images = imageQuery.data?.images ?? []
  const detections = detectionQuery.data?.detections ?? []
  const reports = reportQuery.data?.reports ?? []
  const activity =
    userRole === 'DOCTOR' || userRole === 'ADMIN' ? auditQuery.data?.logs ?? [] : []

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
          {session.user.name}
        </h1>
        <p className="text-gray-600 mt-2">
          {userRole === 'PATIENT' && (isZh ? '管理你的 X 光影像并查看检测结果' : 'Manage your X-ray images and view detection results')}
          {userRole === 'DOCTOR' && (isZh ? '审阅 AI 检测结果并管理患者报告' : 'Review AI detections and manage patient reports')}
          {userRole === 'ADMIN' && (isZh ? '系统总览与管理' : 'System overview and management')}
        </p>
      </div>

      {userRole === 'PATIENT' && (
        <div className="grid md:grid-cols-3 gap-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '我的影像' : 'My Images'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-blue-600 mb-2">{images.length}</div>
              <p className="text-sm text-gray-600 mb-4">{isZh ? '累计上传影像' : 'Total uploaded images'}</p>
              <Link href="/dashboard/images">
                <Button variant="outline" size="sm" className="w-full">
                  {isZh ? '查看全部' : 'View All'}
                </Button>
              </Link>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '待处理' : 'Pending Reviews'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-yellow-600 mb-2">{pendingImages}</div>
              <p className="text-sm text-gray-600 mb-4">{isZh ? '等待 AI 分析' : 'Awaiting AI analysis'}</p>
              <Link href="/dashboard/upload">
                <Button size="sm" className="w-full">
                  {isZh ? '上传新影像' : 'Upload New'}
                </Button>
              </Link>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '报告' : 'Reports'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-green-600 mb-2">{reports.length}</div>
              <p className="text-sm text-gray-600 mb-4">{isZh ? '可查看报告数' : 'Available reports'}</p>
              <Link href="/dashboard/reports">
                <Button variant="outline" size="sm" className="w-full">
                  {isZh ? '查看报告' : 'View Reports'}
                </Button>
              </Link>
            </CardContent>
          </Card>
        </div>
      )}

      {userRole === 'DOCTOR' && (
        <div className="grid md:grid-cols-4 gap-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '待审队列' : 'Review Queue'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-orange-600 mb-2">{pendingDetections}</div>
              <p className="text-sm text-gray-600 mb-4">{isZh ? '待审条目' : 'Pending reviews'}</p>
              <Link href="/dashboard/doctor/queue">
                <Button size="sm" className="w-full">
                  {isZh ? '开始审阅' : 'Start Review'}
                </Button>
              </Link>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '今日已审阅' : 'Today Reviews'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-blue-600 mb-2">{reviewedToday}</div>
              <p className="text-sm text-gray-600 mb-4">{isZh ? '今日完成' : 'Completed today'}</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '报告' : 'Reports'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-purple-600 mb-2">{reports.length}</div>
              <p className="text-sm text-gray-600 mb-4">{isZh ? '报告总数' : 'Total reports'}</p>
              <Link href="/dashboard/reports">
                <Button variant="outline" size="sm" className="w-full">
                  {isZh ? '查看报告' : 'View Reports'}
                </Button>
              </Link>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '高风险待审' : 'High Risk Pending'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-red-600 mb-2">
                {detections.filter((item) => item.status === 'PENDING' && item.cancerProbability >= 0.7).length}
              </div>
              <p className="text-sm text-gray-600 mb-4">{isZh ? '需优先审阅' : 'Need priority review'}</p>
            </CardContent>
          </Card>
        </div>
      )}

      {userRole === 'ADMIN' && (
        <div className="grid md:grid-cols-4 gap-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '用户总数' : 'Total Users'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-blue-600 mb-2">-</div>
              <p className="text-sm text-gray-600">{isZh ? '请查看管理分析页' : 'See Admin Analytics page'}</p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '影像总数' : 'Total Images'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-purple-600 mb-2">{images.length}</div>
              <p className="text-sm text-gray-600">{isZh ? '当前权限可见' : 'Visible in current scope'}</p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '检测数' : 'Detections'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-green-600 mb-2">{detections.length}</div>
              <p className="text-sm text-gray-600">{isZh ? '当前权限可见' : 'Visible in current scope'}</p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">{isZh ? '报告数' : 'Reports'}</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="text-4xl font-bold text-orange-600 mb-2">{reports.length}</div>
              <p className="text-sm text-gray-600">{isZh ? '当前权限可见' : 'Visible in current scope'}</p>
            </CardContent>
          </Card>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>{isZh ? '最近活动' : 'Recent Activity'}</CardTitle>
        </CardHeader>
        <CardContent>
          {activity.length === 0 ? (
            <div className="text-center py-8 text-gray-500">{isZh ? '暂无最近活动' : 'No recent activity'}</div>
          ) : (
            <div className="space-y-3">
              {activity.map((item) => (
                <div key={item.id} className="p-3 bg-gray-50 rounded">
                  <div className="text-sm font-medium">
                    {item.action} [{item.result}]
                  </div>
                  <div className="text-xs text-gray-600 mt-1">
                    {item.entityType}:{item.entityId}
                  </div>
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
