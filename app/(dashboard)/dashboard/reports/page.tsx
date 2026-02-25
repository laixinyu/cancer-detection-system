'use client'

import { useEffect, useRef, useState } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { formatDate } from '@/lib/utils'
import { useI18n } from '@/components/i18n-provider'
import { getInfectionCoverageLabel } from '@/lib/screening'
import { useAuth } from '@/components/auth-provider'

type ReportContent = {
  diagnosis?: string
  findings?: string[]
  recommendations?: string
  screeningSummary?: {
    pneumoniaScore?: number
    lesionScore?: number
    whiteLungScore?: number
    overallScore?: number
    triagePriority?: string
    infectionCoverage?: Record<string, number>
  }
}

type ReportItem = {
  id: string
  status: string
  createdAt: string
  content: unknown
  patient: { user: { name: string; email: string } }
  doctor: { name: string }
  detection: { cancerProbability: number; image: { originalName: string } }
}

function normalizeReportContent(content: unknown): ReportContent {
  if (!content || typeof content !== 'object') return {}
  const c = content as Record<string, unknown>
  return {
    diagnosis: typeof c.diagnosis === 'string' ? c.diagnosis : undefined,
    findings: Array.isArray(c.findings) ? c.findings.filter((item): item is string => typeof item === 'string') : undefined,
    recommendations: typeof c.recommendations === 'string' ? c.recommendations : undefined,
    screeningSummary: c.screeningSummary && typeof c.screeningSummary === 'object' ? (c.screeningSummary as ReportContent['screeningSummary']) : undefined,
  }
}

export default function ReportsPage() {
  const { locale } = useI18n()
  const isZh = locale === 'zh'
  const { status, authFetch } = useAuth()
  const [selectedReportId, setSelectedReportId] = useState<string | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState('')
  const [reports, setReports] = useState<ReportItem[]>([])
  const [exportingPDF, setExportingPDF] = useState(false)
  const [exportError, setExportError] = useState<string | null>(null)
  const reportRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (status !== 'authenticated') return
    let cancelled = false
    const load = async () => {
      setIsLoading(true)
      setError('')
      try {
        const response = await authFetch('/api/v1/reports?limit=100')
        const data = await response.json()
        if (!response.ok) throw new Error(typeof data?.error === 'string' ? data.error : 'Load failed')
        if (!cancelled) setReports(Array.isArray(data.reports) ? data.reports : [])
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : 'Load failed')
      } finally {
        if (!cancelled) setIsLoading(false)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [status, authFetch])

  const selectedReport = reports.find((report) => report.id === selectedReportId) ?? reports[0] ?? null
  const reportContent = normalizeReportContent(selectedReport?.content)

  const exportToPDF = async () => {
    if (!reportRef.current || !selectedReport) return
    setExportingPDF(true)
    setExportError(null)
    try {
      const [{ default: html2canvas }, jspdfModule] = await Promise.all([import('html2canvas'), import('jspdf')])
      const JsPdfCtor = jspdfModule.jsPDF ?? jspdfModule.default
      if (!JsPdfCtor) throw new Error('jsPDF constructor is unavailable')
      const target = reportRef.current
      const canvas = await html2canvas(target, { scale: 2, useCORS: true, backgroundColor: '#ffffff' })
      const imgData = canvas.toDataURL('image/png')
      const pdf = new JsPdfCtor({ orientation: 'p', unit: 'mm', format: 'a4' })
      const pdfWidth = pdf.internal.pageSize.getWidth()
      const pdfHeight = pdf.internal.pageSize.getHeight()
      const imageHeightMm = (canvas.height * pdfWidth) / canvas.width
      let heightLeft = imageHeightMm
      let y = 0
      pdf.addImage(imgData, 'PNG', 0, y, pdfWidth, imageHeightMm)
      heightLeft -= pdfHeight
      while (heightLeft > 0) {
        y = heightLeft - imageHeightMm
        pdf.addPage()
        pdf.addImage(imgData, 'PNG', 0, y, pdfWidth, imageHeightMm)
        heightLeft -= pdfHeight
      }
      pdf.save(`report_${selectedReport.id}_${Date.now()}.pdf`)
    } catch (e) {
      setExportError(e instanceof Error ? e.message : isZh ? '导出失败，请重试' : 'Export failed, please retry')
    } finally {
      setExportingPDF(false)
    }
  }

  const getRiskBadge = (probability: number) => {
    if (probability < 0.3) return { text: isZh ? '低' : 'Low', class: 'bg-green-100 text-green-700' }
    if (probability < 0.7) return { text: isZh ? '中' : 'Medium', class: 'bg-yellow-100 text-yellow-700' }
    return { text: isZh ? '高' : 'High', class: 'bg-red-100 text-red-700' }
  }

  return (
    <div className="space-y-6">
      <div><h1 className="text-3xl font-bold text-gray-900">{isZh ? '医疗报告' : 'Medical Reports'}</h1></div>
      <div className="grid md:grid-cols-2 gap-6">
        <Card><CardHeader><CardTitle>{isZh ? `报告（${reports.length}）` : `Reports (${reports.length})`}</CardTitle></CardHeader><CardContent>
          {isLoading && <div className="text-center py-8 text-gray-500">{isZh ? '正在加载报告...' : 'Loading reports...'}</div>}
          {error && <div className="text-center py-8 text-red-600">{error}</div>}
          {!isLoading && !error && reports.length === 0 && <div className="text-center py-8 text-gray-500">{isZh ? '暂无报告' : 'No reports available'}</div>}
          {!isLoading && !error && reports.length > 0 && <div className="space-y-3">{reports.map((report) => {
            const risk = getRiskBadge(report.detection.cancerProbability)
            return (
              <button key={report.id} type="button" className={`w-full text-left p-4 border rounded-lg transition-colors ${selectedReport?.id === report.id ? 'border-blue-500 bg-blue-50' : 'border-gray-200 hover:border-gray-300'}`} onClick={() => setSelectedReportId(report.id)}>
                <div className="flex items-start justify-between mb-2"><div><h3 className="font-semibold">{report.patient.user.name}</h3><p className="text-sm text-gray-600">{report.detection.image.originalName}</p></div><span className={`px-2 py-1 rounded text-xs font-medium ${report.status === 'FINALIZED' ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-700'}`}>{report.status}</span></div>
                <div className="flex items-center gap-4 text-sm text-gray-600"><span>{report.doctor.name}</span><span>•</span><span>{formatDate(report.createdAt)}</span><span>•</span><span className={`px-2 py-0.5 rounded ${risk.class}`}>{risk.text}{isZh ? '风险' : ' Risk'}</span></div>
              </button>
            )
          })}</div>}
        </CardContent></Card>
        <div>
          {selectedReport ? (
            <Card>
              <CardHeader className="flex flex-row items-center justify-between"><CardTitle>{isZh ? '报告预览' : 'Report Preview'}</CardTitle><Button size="sm" onClick={exportToPDF} disabled={exportingPDF}>{exportingPDF ? (isZh ? '导出中...' : 'Exporting...') : isZh ? '导出 PDF' : 'Export PDF'}</Button></CardHeader>
              <CardContent>
                {exportError && <div className="mb-3 rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{exportError}</div>}
                <div ref={reportRef} className="bg-white p-8 space-y-6">
                  <div className="text-center border-b pb-6"><h1 className="text-2xl font-bold text-blue-600">{isZh ? '医学影像报告' : 'MEDICAL IMAGING REPORT'}</h1></div>
                  <div><h3 className="font-semibold text-lg mb-3">{isZh ? '患者信息' : 'Patient Information'}</h3><div className="grid grid-cols-2 gap-2 text-sm"><div><span className="text-gray-600">{isZh ? '姓名：' : 'Name:'}</span> <span className="font-medium">{selectedReport.patient.user.name}</span></div><div><span className="text-gray-600">{isZh ? '邮箱：' : 'Email:'}</span> <span className="font-medium">{selectedReport.patient.user.email}</span></div><div><span className="text-gray-600">{isZh ? '报告日期：' : 'Report Date:'}</span> <span className="font-medium">{formatDate(selectedReport.createdAt)}</span></div><div><span className="text-gray-600">{isZh ? '报告ID：' : 'Report ID:'}</span> <span className="font-medium">{selectedReport.id}</span></div></div></div>
                  <div className="bg-gray-50 p-4 rounded">
                    <h3 className="font-semibold text-lg mb-3">{isZh ? 'AI 分析' : 'AI Analysis'}</h3>
                    <div className="text-sm space-y-2">
                      <p><span className="text-gray-600">{isZh ? '癌症概率：' : 'Cancer Probability:'}</span> <span className="font-bold text-red-600">{(selectedReport.detection.cancerProbability * 100).toFixed(1)}%</span></p>
                      {reportContent.screeningSummary?.infectionCoverage && (
                        <div>{Object.entries(reportContent.screeningSummary.infectionCoverage).sort((a, b) => b[1] - a[1]).slice(0, 3).map(([label, score]) => (<div key={label} className="text-xs flex items-center justify-between"><span>{getInfectionCoverageLabel(label, isZh)}</span><span className="font-medium">{(score * 100).toFixed(1)}%</span></div>))}</div>
                      )}
                    </div>
                  </div>
                  <div><h3 className="font-semibold text-lg mb-3">{isZh ? '临床诊断' : 'Clinical Diagnosis'}</h3><p className="text-sm">{reportContent.diagnosis ?? (isZh ? '无' : 'N/A')}</p></div>
                </div>
              </CardContent>
            </Card>
          ) : (
            <Card><CardContent className="py-20"><div className="text-center text-gray-500"><div className="text-6xl mb-4">📋</div><p>{isZh ? '选择一份报告查看详情' : 'Select a report to view details'}</p></div></CardContent></Card>
          )}
        </div>
      </div>
    </div>
  )
}
