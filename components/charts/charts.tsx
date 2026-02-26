'use client'

import { LineChart, Line, BarChart, Bar, PieChart, Pie, Cell, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import { useI18n } from '@/components/i18n-provider'

// Activity data (last 7 days)
const activityData = [
  { day: 'Mon', uploads: 12, reviews: 8, reports: 6 },
  { day: 'Tue', uploads: 15, reviews: 12, reports: 10 },
  { day: 'Wed', uploads: 8, reviews: 15, reports: 12 },
  { day: 'Thu', uploads: 20, reviews: 18, reports: 15 },
  { day: 'Fri', uploads: 18, reviews: 16, reports: 14 },
  { day: 'Sat', uploads: 10, reviews: 8, reports: 7 },
  { day: 'Sun', uploads: 6, reviews: 4, reports: 3 },
]

// Detection distribution
const detectionData = [
  { name: 'Low Risk', value: 45, color: '#10b981' },
  { name: 'Medium Risk', value: 30, color: '#f59e0b' },
  { name: 'High Risk', value: 25, color: '#ef4444' },
]

// User growth
const userGrowthData = [
  { month: 'Jan', users: 20 },
  { month: 'Feb', users: 35 },
  { month: 'Mar', users: 50 },
  { month: 'Apr', users: 75 },
  { month: 'May', users: 100 },
  { month: 'Jun', users: 130 },
]

export function ActivityChart() {
  const { t } = useI18n()

  const lineNames = {
    uploads: t('charts.uploads'),
    reviews: t('charts.reviews'),
    reports: t('charts.reports'),
  }

  return (
    <ResponsiveContainer width="100%" height={300}>
      <LineChart data={activityData}>
        <CartesianGrid strokeDasharray="3 3" />
        <XAxis dataKey="day" />
        <YAxis />
        <Tooltip />
        <Legend />
        <Line type="monotone" dataKey="uploads" stroke="#3b82f6" strokeWidth={2} name={lineNames.uploads} />
        <Line type="monotone" dataKey="reviews" stroke="#8b5cf6" strokeWidth={2} name={lineNames.reviews} />
        <Line type="monotone" dataKey="reports" stroke="#10b981" strokeWidth={2} name={lineNames.reports} />
      </LineChart>
    </ResponsiveContainer>
  )
}

export function DetectionChart() {
  const { t } = useI18n()
  const chartData = detectionData.map((item) => ({
    ...item,
    name:
      item.name === 'Low Risk'
        ? t('risk.lowLabel')
        : item.name === 'Medium Risk'
        ? t('risk.mediumLabel')
        : t('risk.highLabel'),
  }))

  return (
    <ResponsiveContainer width="100%" height={300}>
      <PieChart>
        <Pie
          data={chartData}
          cx="50%"
          cy="50%"
          labelLine={false}
          label={({ name, percent }) => `${name}: ${((percent || 0) * 100).toFixed(0)}%`}
          outerRadius={100}
          fill="#8884d8"
          dataKey="value"
        >
          {chartData.map((entry, index) => (
            <Cell key={`cell-${index}`} fill={entry.color} />
          ))}
        </Pie>
        <Tooltip />
      </PieChart>
    </ResponsiveContainer>
  )
}

export function UserGrowthChart() {
  const { t } = useI18n()

  return (
    <ResponsiveContainer width="100%" height={300}>
      <BarChart data={userGrowthData}>
        <CartesianGrid strokeDasharray="3 3" />
        <XAxis dataKey="month" />
        <YAxis />
        <Tooltip />
        <Legend />
        <Bar dataKey="users" fill="#3b82f6" name={t('admin.totalUsers')} />
      </BarChart>
    </ResponsiveContainer>
  )
}

// Model performance over time
const modelPerformanceData = [
  { week: 'Week 1', accuracy: 88, falsePositive: 8, falseNegative: 4 },
  { week: 'Week 2', accuracy: 89, falsePositive: 7.5, falseNegative: 3.5 },
  { week: 'Week 3', accuracy: 91, falsePositive: 6, falseNegative: 3 },
  { week: 'Week 4', accuracy: 92.5, falsePositive: 5.2, falseNegative: 2.3 },
]

export function ModelPerformanceChart() {
  const { t } = useI18n()

  return (
    <ResponsiveContainer width="100%" height={300}>
      <LineChart data={modelPerformanceData}>
        <CartesianGrid strokeDasharray="3 3" />
        <XAxis dataKey="week" />
        <YAxis />
        <Tooltip />
        <Legend />
        <Line type="monotone" dataKey="accuracy" stroke="#10b981" strokeWidth={2} name={t('charts.accuracy')} />
        <Line type="monotone" dataKey="falsePositive" stroke="#f59e0b" strokeWidth={2} name={t('charts.falsePositive')} />
        <Line type="monotone" dataKey="falseNegative" stroke="#ef4444" strokeWidth={2} name={t('charts.falseNegative')} />
      </LineChart>
    </ResponsiveContainer>
  )
}
