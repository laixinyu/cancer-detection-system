'use client'

import { useRef, useState } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { formatDate } from '@/lib/utils'
import jsPDF from 'jspdf'
import html2canvas from 'html2canvas'
import { api } from '@/lib/trpc'
import { useI18n } from '@/components/i18n-provider'

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

function normalizeReportContent(content: unknown): ReportContent {
  if (!content || typeof content !== 'object') {
    return {}
  }

  const c = content as Record<string, unknown>
  return {
    diagnosis: typeof c.diagnosis === 'string' ? c.diagnosis : undefined,
    findings: Array.isArray(c.findings)
      ? c.findings.filter((item): item is string => typeof item === 'string')
      : undefined,
    recommendations:
      typeof c.recommendations === 'string' ? c.recommendations : undefined,
    screeningSummary:
      c.screeningSummary && typeof c.screeningSummary === 'object'
        ? (c.screeningSummary as ReportContent['screeningSummary'])
        : undefined,
  }
}

export default function ReportsPage() {
  const { locale } = useI18n()
  const isZh = locale === 'zh'
  const [selectedReportId, setSelectedReportId] = useState<string | null>(null)
  const [exportingPDF, setExportingPDF] = useState(false)
  const reportRef = useRef<HTMLDivElement>(null)

  const { data, isLoading, error } = api.report.list.useQuery({
    limit: 100,
  })

  const reports = data?.reports ?? []
  const selectedReport =
    reports.find((report) => report.id === selectedReportId) ?? reports[0] ?? null
  const reportContent = normalizeReportContent(selectedReport?.content)

  const exportToPDF = async () => {
    if (!reportRef.current || !selectedReport) return

    setExportingPDF(true)
    try {
      const canvas = await html2canvas(reportRef.current, {
        scale: 2,
        useCORS: true,
      })

      const imgData = canvas.toDataURL('image/png')
      const pdf = new jsPDF('p', 'mm', 'a4')
      const pdfWidth = pdf.internal.pageSize.getWidth()
      const pdfHeight = (canvas.height * pdfWidth) / canvas.width

      pdf.addImage(imgData, 'PNG', 0, 0, pdfWidth, pdfHeight)
      pdf.save(`report_${selectedReport.id}_${Date.now()}.pdf`)
    } catch (e) {
      console.error('Error generating PDF:', e)
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
      <div>
        <h1 className="text-3xl font-bold text-gray-900">{isZh ? '医疗报告' : 'Medical Reports'}</h1>
        <p className="text-gray-600 mt-2">{isZh ? '查看并下载检测报告' : 'View and download detection reports'}</p>
      </div>

      <div className="grid md:grid-cols-2 gap-6">
        <div>
          <Card>
            <CardHeader>
              <CardTitle>{isZh ? `报告（${reports.length}）` : `Reports (${reports.length})`}</CardTitle>
            </CardHeader>
            <CardContent>
              {isLoading && <div className="text-center py-8 text-gray-500">{isZh ? '正在加载报告...' : 'Loading reports...'}</div>}
              {error && <div className="text-center py-8 text-red-600">{error.message}</div>}

              {!isLoading && !error && reports.length === 0 && (
                <div className="text-center py-8 text-gray-500">{isZh ? '暂无报告' : 'No reports available'}</div>
              )}

              {!isLoading && !error && reports.length > 0 && (
                <div className="space-y-3">
                  {reports.map((report) => {
                    const risk = getRiskBadge(report.detection.cancerProbability)
                    return (
                      <button
                        key={report.id}
                        type="button"
                        className={`w-full text-left p-4 border rounded-lg transition-colors ${
                          selectedReport?.id === report.id
                            ? 'border-blue-500 bg-blue-50'
                            : 'border-gray-200 hover:border-gray-300'
                        }`}
                        onClick={() => setSelectedReportId(report.id)}
                      >
                        <div className="flex items-start justify-between mb-2">
                          <div>
                            <h3 className="font-semibold">{report.patient.user.name}</h3>
                            <p className="text-sm text-gray-600">{report.detection.image.originalName}</p>
                          </div>
                          <span
                            className={`px-2 py-1 rounded text-xs font-medium ${
                              report.status === 'FINALIZED'
                                ? 'bg-green-100 text-green-700'
                                : 'bg-gray-100 text-gray-700'
                            }`}
                          >
                            {report.status === 'FINALIZED'
                              ? isZh
                                ? '已定稿'
                                : 'FINALIZED'
                              : isZh
                              ? '草稿'
                              : 'DRAFT'}
                          </span>
                        </div>

                        <div className="flex items-center gap-4 text-sm text-gray-600">
                          <span>{report.doctor.name}</span>
                          <span>•</span>
                          <span>{formatDate(report.createdAt)}</span>
                          <span>•</span>
                          <span className={`px-2 py-0.5 rounded ${risk.class}`}>{risk.text}{isZh ? '风险' : ' Risk'}</span>
                        </div>
                      </button>
                    )
                  })}
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        <div>
          {selectedReport ? (
            <Card>
              <CardHeader className="flex flex-row items-center justify-between">
                <CardTitle>{isZh ? '报告预览' : 'Report Preview'}</CardTitle>
                <Button size="sm" onClick={exportToPDF} disabled={exportingPDF}>
                  {exportingPDF ? (isZh ? '导出中...' : 'Exporting...') : isZh ? '导出 PDF' : 'Export PDF'}
                </Button>
              </CardHeader>
              <CardContent>
                <div ref={reportRef} className="bg-white p-8 space-y-6">
                  <div className="text-center border-b pb-6">
                    <h1 className="text-2xl font-bold text-blue-600">{isZh ? '医学影像报告' : 'MEDICAL IMAGING REPORT'}</h1>
                    <p className="text-sm text-gray-600 mt-2">{isZh ? '肺部检测系统' : 'Lung Detection System'}</p>
                  </div>

                  <div>
                    <h3 className="font-semibold text-lg mb-3">{isZh ? '患者信息' : 'Patient Information'}</h3>
                    <div className="grid grid-cols-2 gap-2 text-sm">
                      <div>
                        <span className="text-gray-600">{isZh ? '姓名：' : 'Name:'}</span>{' '}
                        <span className="font-medium">{selectedReport.patient.user.name}</span>
                      </div>
                      <div>
                        <span className="text-gray-600">{isZh ? '邮箱：' : 'Email:'}</span>{' '}
                        <span className="font-medium">{selectedReport.patient.user.email}</span>
                      </div>
                      <div>
                        <span className="text-gray-600">{isZh ? '报告日期：' : 'Report Date:'}</span>{' '}
                        <span className="font-medium">{formatDate(selectedReport.createdAt)}</span>
                      </div>
                      <div>
                        <span className="text-gray-600">{isZh ? '报告ID：' : 'Report ID:'}</span>{' '}
                        <span className="font-medium">{selectedReport.id}</span>
                      </div>
                    </div>
                  </div>

                  <div>
                    <h3 className="font-semibold text-lg mb-3">{isZh ? '检查信息' : 'Examination Details'}</h3>
                    <div className="text-sm space-y-1">
                      <p><span className="text-gray-600">{isZh ? '检查类型：' : 'Examination Type:'}</span> {isZh ? '胸部X光' : 'Chest X-ray'}</p>
                      <p><span className="text-gray-600">{isZh ? '影像文件：' : 'Image File:'}</span> {selectedReport.detection.image.originalName}</p>
                      <p><span className="text-gray-600">{isZh ? '审阅医生：' : 'Reviewing Physician:'}</span> {selectedReport.doctor.name}</p>
                    </div>
                  </div>

                  <div className="bg-gray-50 p-4 rounded">
                    <h3 className="font-semibold text-lg mb-3">{isZh ? 'AI 分析' : 'AI Analysis'}</h3>
                    <div className="text-sm space-y-2">
                      <p>
                        <span className="text-gray-600">{isZh ? '癌症概率：' : 'Cancer Probability:'}</span>{' '}
                        <span className="font-bold text-red-600">
                          {(selectedReport.detection.cancerProbability * 100).toFixed(1)}%
                        </span>
                      </p>
                      {reportContent.screeningSummary && (
                        <>
                          <p>
                            <span className="text-gray-600">{isZh ? '肺炎风险：' : 'Pneumonia Risk:'}</span>{' '}
                            <span className="font-medium">
                              {((reportContent.screeningSummary.pneumoniaScore ?? 0) * 100).toFixed(1)}%
                            </span>
                          </p>
                          <p>
                            <span className="text-gray-600">{isZh ? '病灶风险：' : 'Lesion Risk:'}</span>{' '}
                            <span className="font-medium">
                              {((reportContent.screeningSummary.lesionScore ?? 0) * 100).toFixed(1)}%
                            </span>
                          </p>
                          <p>
                            <span className="text-gray-600">{isZh ? '白肺评分：' : 'White Lung Score:'}</span>{' '}
                            <span className="font-medium">
                              {((reportContent.screeningSummary.whiteLungScore ?? 0) * 100).toFixed(1)}%
                            </span>
                          </p>
                          <p>
                            <span className="text-gray-600">{isZh ? '分诊优先级：' : 'Triage Priority:'}</span>{' '}
                            <span className="font-medium">
                              {reportContent.screeningSummary.triagePriority ?? 'N/A'}
                            </span>
                          </p>
                          {reportContent.screeningSummary.infectionCoverage && (
                            <div>
                              <span className="text-gray-600">{isZh ? '感染覆盖：' : 'Infection Coverage:'}</span>
                              <div className="mt-1 space-y-1">
                                {Object.entries(reportContent.screeningSummary.infectionCoverage)
                                  .sort((a, b) => b[1] - a[1])
                                  .slice(0, 3)
                                  .map(([label, score]) => (
                                    <div key={label} className="text-xs flex items-center justify-between">
                                      <span>{label}</span>
                                      <span className="font-medium">{(score * 100).toFixed(1)}%</span>
                                    </div>
                                  ))}
                              </div>
                            </div>
                          )}
                        </>
                      )}
                    </div>
                  </div>

                  <div>
                    <h3 className="font-semibold text-lg mb-3">{isZh ? '临床诊断' : 'Clinical Diagnosis'}</h3>
                    <p className="text-sm">{reportContent.diagnosis ?? (isZh ? '无' : 'N/A')}</p>
                  </div>

                  <div>
                    <h3 className="font-semibold text-lg mb-3">{isZh ? '发现项' : 'Findings'}</h3>
                    <ul className="list-disc list-inside text-sm space-y-1">
                      {(reportContent.findings ?? [isZh ? '暂无结构化发现项' : 'No structured findings']).map((finding, idx) => (
                        <li key={idx}>{finding}</li>
                      ))}
                    </ul>
                  </div>

                  <div>
                    <h3 className="font-semibold text-lg mb-3">{isZh ? '建议' : 'Recommendations'}</h3>
                    <p className="text-sm">{reportContent.recommendations ?? (isZh ? '无' : 'N/A')}</p>
                  </div>
                </div>
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardContent className="py-20">
                <div className="text-center text-gray-500">
                  <div className="text-6xl mb-4">📋</div>
                  <p>{isZh ? '选择一份报告查看详情' : 'Select a report to view details'}</p>
                </div>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
