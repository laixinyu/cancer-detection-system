'use client'

import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import { formatMessage, Locale } from '@/lib/i18n'

type I18nContextValue = {
  locale: Locale
  setLocale: (next: Locale) => void
  t: (key: string, vars?: Record<string, string | number>) => string
}

const I18nContext = createContext<I18nContextValue | null>(null)
const STORAGE_KEY = 'xray_locale'

function detectDefaultLocale(): Locale {
  if (typeof window === 'undefined') {
    return 'en'
  }
  const saved = window.localStorage.getItem(STORAGE_KEY)
  if (saved === 'en' || saved === 'zh') {
    return saved
  }
  const browserLang = window.navigator.language.toLowerCase()
  return browserLang.startsWith('zh') ? 'zh' : 'en'
}

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(() => detectDefaultLocale())

  useEffect(() => {
    document.documentElement.lang = locale
    window.localStorage.setItem(STORAGE_KEY, locale)
  }, [locale])

  const value = useMemo<I18nContextValue>(() => {
    return {
      locale,
      setLocale: setLocaleState,
      t: (key, vars) => formatMessage(locale, key, vars),
    }
  }, [locale])

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n() {
  const ctx = useContext(I18nContext)
  if (!ctx) {
    throw new Error('useI18n must be used within I18nProvider')
  }
  return ctx
}
