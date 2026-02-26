export type Locale = 'en' | 'zh'

export type Dictionary = Record<string, string>

const dictionaryLoaders: Record<Locale, () => Promise<Dictionary>> = {
  en: async () => (await import('./i18n/locales/en')).default as Dictionary,
  zh: async () => (await import('./i18n/locales/zh')).default as Dictionary,
}

const dictionaryCache: Partial<Record<Locale, Dictionary>> = {}

export async function loadDictionary(locale: Locale): Promise<Dictionary> {
  const cached = dictionaryCache[locale]
  if (cached) return cached

  const dictionary = await dictionaryLoaders[locale]()
  dictionaryCache[locale] = dictionary
  return dictionary
}

export function getCachedDictionary(locale: Locale): Dictionary | undefined {
  return dictionaryCache[locale]
}

export function formatMessage(
  dictionary: Dictionary | undefined,
  key: string,
  vars?: Record<string, string | number>,
  fallbackDictionary?: Dictionary
) {
  const fallback = fallbackDictionary?.[key] ?? key
  const template = dictionary?.[key] ?? fallback
  if (!vars) return template

  return Object.entries(vars).reduce((acc, [name, value]) => {
    return acc.replaceAll(`{${name}}`, String(value))
  }, template)
}
