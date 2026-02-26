'use client'

import { createContext, useContext, useEffect, useMemo, useState } from 'react'
import {
  Dictionary,
  formatMessage,
  getCachedDictionary,
  loadDictionary,
  Locale,
} from '@/lib/i18n'

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
  const [dictionary, setDictionary] = useState<Dictionary | undefined>(() =>
    getCachedDictionary(locale)
  )
  const [fallbackDictionary, setFallbackDictionary] = useState<
    Dictionary | undefined
  >(() => getCachedDictionary('en'))

  useEffect(() => {
    document.documentElement.lang = locale
    window.localStorage.setItem(STORAGE_KEY, locale)
  }, [locale])

  useEffect(() => {
    let active = true

    const load = async () => {
      const [current, fallback] = await Promise.all([
        loadDictionary(locale),
        loadDictionary('en'),
      ])
      if (!active) return
      setDictionary(current)
      setFallbackDictionary(fallback)
    }

    void load()

    return () => {
      active = false
    }
  }, [locale])

  const value = useMemo<I18nContextValue>(() => {
    return {
      locale,
      setLocale: setLocaleState,
      t: (key, vars) => formatMessage(dictionary, key, vars, fallbackDictionary),
    }
  }, [dictionary, fallbackDictionary, locale])

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n() {
  const ctx = useContext(I18nContext)
  if (!ctx) {
    throw new Error('useI18n must be used within I18nProvider')
  }
  return ctx
}
