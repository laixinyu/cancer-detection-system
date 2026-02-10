'use client'

import ImageUpload from '@/components/image-upload'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useI18n } from '@/components/i18n-provider'

export default function UploadPage() {
  const { t } = useI18n()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold text-gray-900">{t('upload.title')}</h1>
        <p className="text-gray-600 mt-2">
          {t('upload.subtitle')}
        </p>
        <div className="mt-3 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900">
          {t('upload.researchNotice')}
        </div>
      </div>

      <div className="grid md:grid-cols-3 gap-6">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">📋 {t('upload.step1')}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-gray-600">
              {t('upload.step1Desc')}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">🤖 {t('upload.step2')}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-gray-600">
              {t('upload.step2Desc')}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">📊 {t('upload.step3')}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-gray-600">
              {t('upload.step3Desc')}
            </p>
          </CardContent>
        </Card>
      </div>

      <ImageUpload />
    </div>
  )
}
