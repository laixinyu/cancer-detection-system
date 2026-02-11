'use client'

import { useState, useCallback } from 'react'
import { useDropzone } from 'react-dropzone'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { useI18n } from '@/components/i18n-provider'

interface UploadedFile {
  file: File
  preview: string
  progress: number
  status: 'pending' | 'uploading' | 'success' | 'error'
  error?: string
}

export default function ImageUpload() {
  const { t, locale } = useI18n()
  const isZh = locale === 'zh'
  const [files, setFiles] = useState<UploadedFile[]>([])
  const [consentAccepted, setConsentAccepted] = useState(false)
  const consentVersion = 'v1.0'

  const onDrop = useCallback((acceptedFiles: File[]) => {
    const newFiles = acceptedFiles.map(file => ({
      file,
      preview: URL.createObjectURL(file),
      progress: 0,
      status: 'pending' as const,
    }))
    setFiles(prev => [...prev, ...newFiles])
  }, [])

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    accept: {
      'image/*': ['.png', '.jpg', '.jpeg', '.dcm', '.tiff']
    },
    maxFiles: 10,
    maxSize: 10485760, // 10MB
  })

  const handleUpload = async () => {
    for (let i = 0; i < files.length; i++) {
      if (files[i].status !== 'pending') continue

      setFiles(prev => prev.map((f, idx) => 
        idx === i ? { ...f, status: 'uploading' as const } : f
      ))

      try {
        const formData = new FormData()
        formData.append('file', files[i].file)
        formData.append('consentAccepted', String(consentAccepted))
        formData.append('consentVersion', consentVersion)

        setFiles(prev => prev.map((f, idx) => 
          idx === i ? { ...f, progress: 30 } : f
        ))

        // Upload to API
        const response = await fetch('/api/images/upload', {
          method: 'POST',
          body: formData,
        })

        const result = await response.json().catch(() => null)

        if (!response.ok) {
          const message =
            (result && typeof result.error === 'string' && result.error) ||
            (result && typeof result.details === 'string' && result.details) ||
            t('imageUpload.uploadFailed')
          throw new Error(message)
        }

        if (!result?.image || result.image.status !== 'COMPLETED') {
          throw new Error(
            isZh
              ? 'AI 分析未完成，请稍后重试上传'
              : 'AI analysis did not complete. Please retry upload.'
          )
        }

        setFiles(prev => prev.map((f, idx) => 
          idx === i ? { ...f, progress: 100 } : f
        ))

        setFiles(prev => prev.map((f, idx) => 
          idx === i ? { ...f, status: 'success' as const, progress: 100 } : f
        ))
      } catch (error) {
        const errorMessage =
          error instanceof Error && error.message
            ? error.message
            : t('imageUpload.uploadFailed')
        setFiles(prev => prev.map((f, idx) => 
          idx === i ? { 
            ...f, 
            status: 'error' as const, 
            error: errorMessage
          } : f
        ))
      }
    }
  }

  const removeFile = (index: number) => {
    URL.revokeObjectURL(files[index].preview)
    setFiles(prev => prev.filter((_, idx) => idx !== index))
  }

  const formatFileSize = (bytes: number) => {
    if (bytes === 0) return '0 Bytes'
    const k = 1024
    const sizes = ['Bytes', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i]
  }

  return (
    <div className="space-y-6">
      {/* Upload Area */}
      <Card className="p-8">
        <div
          {...getRootProps()}
          className={`
            border-2 border-dashed rounded-lg p-12 text-center cursor-pointer transition-colors
            ${isDragActive ? 'border-blue-500 bg-blue-50' : 'border-gray-300 hover:border-blue-400'}
          `}
        >
          <input {...getInputProps()} />
          <div className="text-6xl mb-4">📤</div>
          {isDragActive ? (
            <p className="text-lg text-blue-600">{t('imageUpload.dropActive')}</p>
          ) : (
            <div>
              <p className="text-lg font-medium text-gray-700 mb-2">
                {t('imageUpload.dragDrop')}
              </p>
              <p className="text-sm text-gray-500 mb-4">
                {t('imageUpload.orClick')}
              </p>
              <p className="text-xs text-gray-400">
                {t('imageUpload.formats')}
              </p>
            </div>
          )}
        </div>
      </Card>

      {/* File List */}
      {files.length > 0 && (
        <Card className="p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold">
              {t('imageUpload.fileCount', { count: files.length })}
            </h3>
            <Button 
              onClick={handleUpload}
              disabled={files.every(f => f.status !== 'pending') || !consentAccepted}
            >
              {t('imageUpload.uploadAll')}
            </Button>
          </div>

          <label className="mb-4 flex items-start gap-2 text-sm text-gray-700">
            <input
              type="checkbox"
              checked={consentAccepted}
              onChange={(e) => setConsentAccepted(e.target.checked)}
              className="mt-1"
            />
            <span>
              {t('imageUpload.consent', { version: consentVersion })}
            </span>
          </label>

          <div className="space-y-4">
            {files.map((file, index) => (
              <div
                key={index}
                className="flex items-center gap-4 p-4 bg-gray-50 rounded-lg"
              >
                {/* Preview */}
                <div className="w-16 h-16 bg-gray-200 rounded overflow-hidden flex-shrink-0">
                  {file.file.type.startsWith('image/') && (
                    <img
                      src={file.preview}
                      alt={file.file.name}
                      className="w-full h-full object-cover"
                    />
                  )}
                </div>

                {/* File Info */}
                <div className="flex-1 min-w-0">
                  <p className="font-medium text-sm truncate">
                    {file.file.name}
                  </p>
                  <p className="text-xs text-gray-500">
                    {formatFileSize(file.file.size)}
                  </p>

                  {/* Progress Bar */}
                  {file.status === 'uploading' && (
                    <div className="mt-2">
                      <div className="w-full bg-gray-200 rounded-full h-2">
                        <div
                          className="bg-blue-600 h-2 rounded-full transition-all"
                          style={{ width: `${file.progress}%` }}
                        />
                      </div>
                    </div>
                  )}

                  {/* Status */}
                  <div className="mt-1">
                    {file.status === 'pending' && (
                      <span className="text-xs text-gray-500">{t('imageUpload.ready')}</span>
                    )}
                    {file.status === 'uploading' && (
                      <span className="text-xs text-blue-600">{t('imageUpload.uploading')}</span>
                    )}
                    {file.status === 'success' && (
                      <span className="text-xs text-green-600">✓ {t('imageUpload.uploaded')}</span>
                    )}
                    {file.status === 'error' && (
                      <span className="text-xs text-red-600">✗ {file.error}</span>
                    )}
                  </div>
                </div>

                {/* Remove Button */}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => removeFile(index)}
                  className="flex-shrink-0"
                >
                  ✕
                </Button>
              </div>
            ))}
          </div>
        </Card>
      )}
    </div>
  )
}
