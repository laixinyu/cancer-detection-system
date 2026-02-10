'use client'

import { Button } from '@/components/ui/button'
import { useI18n } from '@/components/i18n-provider'
import { Locale } from '@/lib/i18n'
import { cn } from '@/lib/utils'

export default function LanguageSwitcher({ compact = false }: { compact?: boolean }) {
  const { locale, setLocale, t } = useI18n()

  const renderItem = (value: Locale, label: string) => (
    <Button
      key={value}
      type="button"
      size="sm"
      variant="outline"
      className={cn(
        'h-8 px-2',
        locale === value ? 'bg-blue-50 border-blue-400 text-blue-700' : ''
      )}
      onClick={() => setLocale(value)}
    >
      {label}
    </Button>
  )

  if (compact) {
    return (
      <div className="flex items-center gap-1">
        {renderItem('en', 'EN')}
        {renderItem('zh', '中')}
      </div>
    )
  }

  return (
    <div className="flex items-center gap-2">
      <span className="text-sm text-gray-600">{t('common.language')}:</span>
      {renderItem('en', t('lang.en'))}
      {renderItem('zh', t('lang.zh'))}
    </div>
  )
}
