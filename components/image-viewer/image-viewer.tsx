'use client'

import { useState, useRef, useEffect, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { useI18n } from '@/components/i18n-provider'

interface AnnotationRect {
  id: string
  x: number
  y: number
  width: number
  height: number
  label: string
  confidence?: number
  color: string
}

interface ImageViewerProps {
  imageUrl: string
  detections?: AnnotationRect[]
  editable?: boolean
  onAnnotationAdd?: (annotation: AnnotationRect) => void
}

export default function ImageViewer({ 
  imageUrl, 
  detections = [],
  editable = false,
  onAnnotationAdd 
}: ImageViewerProps) {
  const { t } = useI18n()
  const [scale, setScale] = useState(1)
  const [position, setPosition] = useState({ x: 0, y: 0 })
  const [dragging, setDragging] = useState(false)
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 })
  const [drawing, setDrawing] = useState(false)
  const [drawStart, setDrawStart] = useState({ x: 0, y: 0 })
  const [currentRect, setCurrentRect] = useState<AnnotationRect | null>(null)
  const [annotations, setAnnotations] = useState<AnnotationRect[]>(detections)
  const [showAnnotations, setShowAnnotations] = useState(true)
  const [brightness, setBrightness] = useState(100)
  const [contrast, setContrast] = useState(100)
  const [imageSize, setImageSize] = useState({ width: 0, height: 0 })
  
  const containerRef = useRef<HTMLDivElement>(null)
  const initializedRef = useRef(false)

  useEffect(() => {
    setAnnotations(detections)
  }, [detections])

  useEffect(() => {
    initializedRef.current = false
  }, [imageUrl])

  const initializeView = useCallback((width: number, height: number) => {
    const container = containerRef.current
    if (!container || width <= 0 || height <= 0) return

    const containerWidth = container.clientWidth
    const containerHeight = container.clientHeight
    if (containerWidth <= 0 || containerHeight <= 0) return

    const fitScale = Math.min(containerWidth / width, containerHeight / height)
    const safeScale = Math.max(Math.min(fitScale, 1), 0.1)
    const centeredX = (containerWidth - width * safeScale) / 2
    const centeredY = (containerHeight - height * safeScale) / 2

    setScale(safeScale)
    setPosition({ x: centeredX, y: centeredY })
    initializedRef.current = true
  }, [])

  const handleZoomIn = () => {
    setScale(prev => Math.min(prev + 0.25, 3))
  }

  const handleZoomOut = () => {
    setScale(prev => Math.max(prev - 0.25, 0.5))
  }

  const handleReset = () => {
    initializeView(imageSize.width, imageSize.height)
    setBrightness(100)
    setContrast(100)
  }

  const getImageCoordinates = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return
    const rawX = (e.clientX - rect.left - position.x) / scale
    const rawY = (e.clientY - rect.top - position.y) / scale
    const x = Math.max(0, Math.min(rawX, imageSize.width))
    const y = Math.max(0, Math.min(rawY, imageSize.height))
    return { x, y }
  }

  const handleMouseDown = (e: React.MouseEvent<HTMLDivElement>) => {
    const coords = getImageCoordinates(e)
    if (!coords) return

    if (editable && e.shiftKey) {
      setDrawing(true)
      setDrawStart(coords)
    } else {
      setDragging(true)
      setDragStart({ x: e.clientX - position.x, y: e.clientY - position.y })
    }
  }

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    if (drawing && editable) {
      const coords = getImageCoordinates(e)
      if (!coords) return

      setCurrentRect({
        id: 'temp',
        x: Math.min(drawStart.x, coords.x),
        y: Math.min(drawStart.y, coords.y),
        width: Math.abs(coords.x - drawStart.x),
        height: Math.abs(coords.y - drawStart.y),
        label: 'New Finding',
        color: '#ef4444',
      })
    } else if (dragging) {
      setPosition({
        x: e.clientX - dragStart.x,
        y: e.clientY - dragStart.y,
      })
    }
  }

  const handleMouseUp = () => {
    if (drawing && currentRect && currentRect.width > 10 && currentRect.height > 10) {
      const newAnnotation = {
        ...currentRect,
        id: Date.now().toString(),
      }
      setAnnotations([...annotations, newAnnotation])
      onAnnotationAdd?.(newAnnotation)
    }
    setDrawing(false)
    setDragging(false)
    setCurrentRect(null)
  }

  const deleteAnnotation = (id: string) => {
    setAnnotations(annotations.filter(a => a.id !== id))
  }

  return (
    <div className="space-y-4">
      {/* Toolbar */}
      <Card className="p-4">
        <div className="flex items-center gap-4 flex-wrap">
          {/* Zoom Controls */}
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{t('viewer.zoom')}:</span>
            <Button size="sm" variant="outline" onClick={handleZoomOut}>
              🔍−
            </Button>
            <span className="text-sm w-16 text-center">{Math.round(scale * 100)}%</span>
            <Button size="sm" variant="outline" onClick={handleZoomIn}>
              🔍+
            </Button>
          </div>

          {/* Brightness */}
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{t('viewer.brightness')}:</span>
            <input
              type="range"
              min="0"
              max="200"
              value={brightness}
              onChange={(e) => setBrightness(Number(e.target.value))}
              className="w-24"
            />
            <span className="text-sm w-12">{brightness}%</span>
          </div>

          {/* Contrast */}
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{t('viewer.contrast')}:</span>
            <input
              type="range"
              min="0"
              max="200"
              value={contrast}
              onChange={(e) => setContrast(Number(e.target.value))}
              className="w-24"
            />
            <span className="text-sm w-12">{contrast}%</span>
          </div>

          {/* Toggle Annotations */}
          <Button
            size="sm"
            variant={showAnnotations ? "default" : "outline"}
            onClick={() => setShowAnnotations(!showAnnotations)}
          >
            {showAnnotations ? `👁️ ${t('viewer.hideAnnotations')}` : `👁️‍🗨️ ${t('viewer.showAnnotations')}`}
          </Button>

          {/* Reset */}
          <Button size="sm" variant="outline" onClick={handleReset}>
            🔄 {t('common.reset')}
          </Button>
        </div>

        {editable && (
          <div className="mt-3 pt-3 border-t">
            <p className="text-sm text-gray-600">
              💡 {t('viewer.hintPrefix')}{' '}
              <kbd className="px-2 py-1 bg-gray-100 rounded">Shift</kbd>{' '}
              {t('viewer.hintSuffix')}
            </p>
          </div>
        )}
      </Card>

      {/* Viewer */}
      <div className="grid md:grid-cols-4 gap-4">
        <div className="md:col-span-3">
          <Card className="p-4 bg-gray-900">
            <div
              ref={containerRef}
              className="relative overflow-hidden bg-black rounded"
              style={{ height: '600px' }}
              onMouseDown={handleMouseDown}
              onMouseMove={handleMouseMove}
              onMouseUp={handleMouseUp}
              onMouseLeave={handleMouseUp}
            >
              {imageSize.width > 0 && imageSize.height > 0 && (
                <div
                  className="absolute cursor-move select-none"
                  style={{
                    width: imageSize.width,
                    height: imageSize.height,
                    transform: `translate(${position.x}px, ${position.y}px) scale(${scale})`,
                    transformOrigin: '0 0',
                  }}
                >
                  <img
                    src={imageUrl}
                    alt="X-ray"
                    className="absolute top-0 left-0"
                    style={{
                      width: imageSize.width,
                      height: imageSize.height,
                      filter: `brightness(${brightness}%) contrast(${contrast}%)`,
                    }}
                    draggable={false}
                    onLoad={(e) => {
                      const target = e.currentTarget
                      const width = target.naturalWidth
                      const height = target.naturalHeight
                      setImageSize({ width, height })
                      if (!initializedRef.current) {
                        initializeView(width, height)
                      }
                    }}
                  />

                  {showAnnotations && (
                    <svg
                      className="absolute top-0 left-0 pointer-events-none"
                      width={imageSize.width}
                      height={imageSize.height}
                      viewBox={`0 0 ${imageSize.width} ${imageSize.height}`}
                    >
                      {annotations.map((ann) => (
                        <g key={ann.id}>
                          <rect
                            x={ann.x}
                            y={ann.y}
                            width={ann.width}
                            height={ann.height}
                            fill="none"
                            stroke={ann.color}
                            strokeWidth={2 / scale}
                            opacity={0.8}
                          />
                          <text
                            x={ann.x}
                            y={ann.y - 5 / scale}
                            fill={ann.color}
                            fontSize={12 / scale}
                            fontWeight="bold"
                          >
                            {ann.label} {ann.confidence && `(${(ann.confidence * 100).toFixed(0)}%)`}
                          </text>
                        </g>
                      ))}
                      {currentRect && (
                        <rect
                          x={currentRect.x}
                          y={currentRect.y}
                          width={currentRect.width}
                          height={currentRect.height}
                          fill="none"
                          stroke={currentRect.color}
                          strokeWidth={2 / scale}
                          opacity={0.6}
                          strokeDasharray="5,5"
                        />
                      )}
                    </svg>
                  )}
                </div>
              )}

              {imageSize.width === 0 && (
                <img
                  src={imageUrl}
                  alt="X-ray preload"
                  className="hidden"
                  onLoad={(e) => {
                    const target = e.currentTarget
                    const width = target.naturalWidth
                    const height = target.naturalHeight
                    setImageSize({ width, height })
                    if (!initializedRef.current) {
                      initializeView(width, height)
                    }
                  }}
                />
              )}

              {imageSize.width === 0 && (
                <div className="absolute inset-0 flex items-center justify-center text-sm text-gray-300">
                  {t('viewer.loadingImage')}
                </div>
              )}
            </div>
          </Card>
        </div>

        {/* Annotations List */}
        <div>
          <Card className="p-4">
            <h3 className="font-semibold mb-3">
              {t('viewer.findings', { count: annotations.length })}
            </h3>
            <div className="space-y-2">
              {annotations.map((ann) => (
                <div
                  key={ann.id}
                  className="p-2 bg-gray-50 rounded text-sm"
                >
                  <div className="flex items-start justify-between">
                    <div>
                      <div className="font-medium">{ann.label}</div>
                      {ann.confidence && (
                        <div className="text-xs text-gray-600">
                          {t('viewer.confidence')}: {(ann.confidence * 100).toFixed(1)}%
                        </div>
                      )}
                    </div>
                    {editable && (
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => deleteAnnotation(ann.id)}
                      >
                        ✕
                      </Button>
                    )}
                  </div>
                  <div className="text-xs text-gray-500 mt-1">
                    {t('viewer.position')}: ({Math.round(ann.x)}, {Math.round(ann.y)})
                  </div>
                </div>
              ))}
              {annotations.length === 0 && (
                <p className="text-sm text-gray-500">{t('viewer.noFindings')}</p>
              )}
            </div>
          </Card>
        </div>
      </div>
    </div>
  )
}
