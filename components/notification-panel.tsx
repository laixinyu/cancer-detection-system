'use client'

import { useMemo, useState } from 'react'
import { useSession } from 'next-auth/react'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/trpc'
import { useI18n } from '@/components/i18n-provider'
import { formatDate } from '@/lib/utils'

interface NotificationItem {
  id: string
  type: 'success' | 'warning' | 'error' | 'info'
  title: string
  message: string
  timestamp: Date
  read: boolean
}

export default function NotificationPanel() {
  const { t } = useI18n()
  const { data: session, status } = useSession()
  const [isOpen, setIsOpen] = useState(false)
  const [readIds, setReadIds] = useState<Set<string>>(new Set())

  const role = session?.user.role
  const isStaff = role === 'DOCTOR' || role === 'ADMIN'

  const auditQuery = api.audit.list.useQuery(
    { limit: 20 },
    { enabled: status === 'authenticated' && isStaff }
  )
  const reportQuery = api.report.list.useQuery(
    { limit: 20 },
    { enabled: status === 'authenticated' && !isStaff }
  )
  const imageQuery = api.image.list.useQuery(
    { limit: 20 },
    { enabled: status === 'authenticated' && !isStaff }
  )

  const notifications = useMemo<NotificationItem[]>(() => {
    if (isStaff) {
      return (auditQuery.data?.logs ?? []).map((log) => {
        const type: NotificationItem['type'] =
          log.result === 'FAILED'
            ? 'error'
            : log.action.includes('HIGH') || log.action.includes('FAILED')
            ? 'warning'
            : 'info'
        return {
          id: `audit-${log.id}`,
          type,
          title: log.action.replaceAll('_', ' '),
          message: `${log.entityType}:${log.entityId}`,
          timestamp: new Date(log.createdAt),
          read: readIds.has(`audit-${log.id}`),
        }
      })
    }

    const reportNotifications = (reportQuery.data?.reports ?? []).map((report) => ({
      id: `report-${report.id}`,
      type: report.status === 'FINALIZED' ? ('success' as const) : ('info' as const),
      title:
        report.status === 'FINALIZED'
          ? t('notify.reportFinalized')
          : t('notify.reportDraftUpdated'),
      message: `Report for ${report.detection.image.originalName}`,
      timestamp: new Date(report.updatedAt),
      read: readIds.has(`report-${report.id}`),
    }))

    const imageNotifications = (imageQuery.data?.images ?? []).map((image) => ({
      id: `image-${image.id}`,
      type:
        image.status === 'FAILED'
          ? ('error' as const)
          : image.status === 'COMPLETED'
          ? ('success' as const)
          : ('info' as const),
      title: t('notify.imageStatus', { status: image.status }),
      message: image.originalName,
      timestamp: new Date(image.updatedAt),
      read: readIds.has(`image-${image.id}`),
    }))

    return [...reportNotifications, ...imageNotifications].sort(
      (a, b) => b.timestamp.getTime() - a.timestamp.getTime()
    )
  }, [isStaff, auditQuery.data?.logs, reportQuery.data?.reports, imageQuery.data?.images, readIds, t])

  const unreadCount = notifications.filter((n) => !n.read).length

  const markAsRead = (id: string) => {
    setReadIds((prev) => new Set([...prev, id]))
  }

  const markAllAsRead = () => {
    setReadIds(new Set(notifications.map((n) => n.id)))
  }

  const getTypeIcon = (type: NotificationItem['type']) => {
    if (type === 'success') return '✅'
    if (type === 'warning') return '⚠️'
    if (type === 'error') return '❌'
    return 'ℹ️'
  }

  return (
    <div className="relative">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="relative p-2 rounded-lg hover:bg-gray-100 transition-colors"
      >
        <span className="text-2xl">🔔</span>
        {unreadCount > 0 && (
          <span className="absolute top-0 right-0 bg-red-500 text-white text-xs rounded-full w-5 h-5 flex items-center justify-center font-bold">
            {unreadCount}
          </span>
        )}
      </button>

      {isOpen && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setIsOpen(false)} />
          <Card className="absolute right-0 top-12 w-96 max-h-[32rem] overflow-hidden shadow-xl z-50">
            <div className="p-4 border-b bg-white sticky top-0">
              <div className="flex items-center justify-between mb-2">
                <h3 className="font-semibold text-lg">{t('common.notifications')}</h3>
                <Button size="sm" variant="ghost" onClick={markAllAsRead} disabled={unreadCount === 0}>
                  {t('notify.markAllRead')}
                </Button>
              </div>
              <p className="text-sm text-gray-600">{t('notify.unread', { count: unreadCount })}</p>
            </div>

            <div className="overflow-y-auto max-h-96">
              {notifications.length === 0 ? (
                <div className="p-8 text-center text-gray-500">
                  <div className="text-4xl mb-2">🔕</div>
                  <p>{t('notify.none')}</p>
                </div>
              ) : (
                <div>
                  {notifications.map((notification) => (
                    <button
                      key={notification.id}
                      type="button"
                      className={`w-full text-left p-4 border-b hover:bg-gray-50 transition-colors ${
                        !notification.read ? 'bg-blue-50' : ''
                      }`}
                      onClick={() => markAsRead(notification.id)}
                    >
                      <div className="flex items-start gap-3">
                        <div className="text-2xl">{getTypeIcon(notification.type)}</div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-start justify-between gap-2">
                            <h4 className="font-semibold text-sm">{notification.title}</h4>
                            {!notification.read && (
                              <span className="w-2 h-2 bg-blue-500 rounded-full flex-shrink-0 mt-1" />
                            )}
                          </div>
                          <p className="text-sm text-gray-600 mt-1">{notification.message}</p>
                          <p className="text-xs text-gray-400 mt-2">{formatDate(notification.timestamp)}</p>
                        </div>
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </Card>
        </>
      )}
    </div>
  )
}
