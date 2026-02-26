'use client'

import { useEffect, useState } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import Link from 'next/link'
import { useSession } from 'next-auth/react'
import { formatDate, formatFileSize } from '@/lib/utils'
import { useI18n } from '@/components/i18n-provider'
import { gatewayGet } from '@/lib/gateway-client'

type ImageItem = {
  id: string
  filePath: string
  originalName: string
  fileType: string
  fileSize: number
  status: string
  createdAt: string
  updatedAt: string
  detections: Array<{ cancerProbability: number }>
}

export default function ImagesPage() {
  const { t } = useI18n()
  const { data: session, status } = useSession()
  const [images, setImages] = useState<ImageItem[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [previewLoadFailed, setPreviewLoadFailed] = useState<Record<string, boolean>>({})
  const [previewImage, setPreviewImage] = useState<{
    filePath: string
    originalName: string
    fileType: string
  } | null>(null)

  useEffect(() => {
    if (status !== 'authenticated' || !session?.user.accessToken) {
      return
    }
    let active = true
    gatewayGet<{ images: ImageItem[] }>('/images', session.user.accessToken, { limit: 100 })
      .then((data) => {
        if (!active) return
        setImages(data.images ?? [])
      })
      .catch((err) => {
        if (!active) return
        setError(err instanceof Error ? err.message : 'Failed to load images')
      })
      .finally(() => {
        if (!active) return
        setIsLoading(false)
      })
    return () => {
      active = false
    }
  }, [status, session?.user.accessToken])

  const completed = images.filter((image) => image.status === 'COMPLETED').length
  const processing = images.filter((image) => image.status === 'PROCESSING').length
  const failed = images.filter((image) => image.status === 'FAILED').length

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'COMPLETED':
        return 'text-green-600 bg-green-100'
      case 'PROCESSING':
        return 'text-blue-600 bg-blue-100'
      case 'FAILED':
        return 'text-red-600 bg-red-100'
      default:
        return 'text-gray-600 bg-gray-100'
    }
  }

  const getRiskLevel = (probability: number) => {
    if (probability < 0.3) return { text: t('risk.low'), color: 'text-green-600' }
    if (probability < 0.7) return { text: t('risk.medium'), color: 'text-yellow-600' }
    return { text: t('risk.high'), color: 'text-red-600' }
  }

  const canInlinePreview = (fileType: string, imageId: string) => {
    if (previewLoadFailed[imageId]) return false
    return fileType !== 'DICOM'
  }

  const openPreview = (image: { filePath: string; originalName: string; fileType: string }) => {
    setPreviewImage(image)
  }

  const closePreview = () => {
    setPreviewImage(null)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">{t('images.title')}</h1>
          <p className="text-gray-600 mt-2">{t('images.subtitle')}</p>
        </div>
        <Link href="/dashboard/upload">
          <Button>{t('images.uploadNew')}</Button>
        </Link>
      </div>

      <div className="grid md:grid-cols-4 gap-4">
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold text-blue-600">{images.length}</div>
            <p className="text-sm text-gray-600">{t('images.total')}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold text-green-600">{completed}</div>
            <p className="text-sm text-gray-600">{t('images.completed')}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold text-yellow-600">{processing}</div>
            <p className="text-sm text-gray-600">{t('images.processing')}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold text-red-600">{failed}</div>
            <p className="text-sm text-gray-600">{t('images.failed')}</p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t('images.uploadedTitle')}</CardTitle>
        </CardHeader>
        <CardContent>
          {(status === 'loading' || isLoading) && (
            <div className="text-center py-8 text-gray-500">{t('images.loading')}</div>
          )}
          {status === 'unauthenticated' && (
            <div className="text-center py-8 text-red-600">{t('images.sessionExpired')}</div>
          )}
          {error && <div className="text-center py-8 text-red-600">{error}</div>}

          {status === 'authenticated' && !isLoading && !error && images.length === 0 && (
            <div className="text-center py-12">
              <div className="text-6xl mb-4">📁</div>
              <p className="text-gray-500 mb-4">{t('images.empty')}</p>
              <Link href="/dashboard/upload">
                <Button>{t('images.uploadFirst')}</Button>
              </Link>
            </div>
          )}

          {status === 'authenticated' && !isLoading && !error && images.length > 0 && (
            <div className="space-y-4">
              {images.map((image) => {
                const latestDetection = image.detections[0]
                const risk =
                  latestDetection && getRiskLevel(latestDetection.cancerProbability)

                return (
                  <div
                    key={image.id}
                    className="flex items-center gap-4 p-4 bg-gray-50 rounded-lg hover:bg-gray-100 transition-colors"
                  >
                    <div className="w-20 h-20 bg-gray-200 rounded overflow-hidden flex items-center justify-center flex-shrink-0">
                      {canInlinePreview(image.fileType, image.id) ? (
                        // Use native img for local /public/uploads preview.
                        <img
                          src={image.filePath}
                          alt={image.originalName}
                          className="w-full h-full object-cover"
                          loading="lazy"
                          onError={() =>
                            setPreviewLoadFailed((prev) => ({
                              ...prev,
                              [image.id]: true,
                            }))
                          }
                        />
                      ) : (
                        <span className="text-3xl">{image.fileType === 'DICOM' ? '🩻' : '🖼️'}</span>
                      )}
                    </div>

                    <div className="flex-1 min-w-0">
                      <h3 className="font-medium truncate">{image.originalName}</h3>
                      <div className="flex items-center gap-4 mt-1 text-sm text-gray-600">
                        <span>{formatFileSize(image.fileSize)}</span>
                        <span>•</span>
                        <span>{formatDate(image.createdAt)}</span>
                      </div>

                      {latestDetection && risk && (
                        <div className="mt-2 flex items-center gap-2">
                          <span className="text-sm text-gray-600">{t('images.lesionRisk')}</span>
                          <span className={`text-sm font-semibold ${risk.color}`}>{risk.text}</span>
                          <span className="text-sm text-gray-500">
                            ({(latestDetection.cancerProbability * 100).toFixed(1)}%)
                          </span>
                        </div>
                      )}
                    </div>

                    <div className="flex items-center gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() =>
                          openPreview({
                            filePath: image.filePath,
                            originalName: image.originalName,
                            fileType: image.fileType,
                          })
                        }
                      >
                        {t('images.preview')}
                      </Button>
                      <span className={`px-3 py-1 rounded-full text-xs font-medium ${getStatusColor(image.status)}`}>
                        {image.status}
                      </span>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {previewImage && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4" onClick={closePreview}>
          <div
            className="w-full max-w-6xl rounded-lg bg-white shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b px-4 py-3">
              <div className="min-w-0">
                <h3 className="truncate text-base font-semibold text-gray-900">{previewImage.originalName}</h3>
                <p className="text-xs text-gray-500">{previewImage.fileType}</p>
              </div>
              <Button variant="outline" size="sm" onClick={closePreview}>
                {t('images.close')}
              </Button>
            </div>
            <div className="flex max-h-[80vh] items-center justify-center bg-black p-3">
              {previewImage.fileType === 'DICOM' ? (
                <div className="text-center text-gray-300">
                  <div className="mb-2 text-5xl">🩻</div>
                  <p>{t('images.dicomNoPreview')}</p>
                </div>
              ) : (
                <img
                  src={previewImage.filePath}
                  alt={previewImage.originalName}
                  className="max-h-[76vh] w-auto max-w-full object-contain"
                />
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
