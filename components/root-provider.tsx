'use client'

import AuthProvider from '@/components/auth-provider'
import { I18nProvider } from '@/components/i18n-provider'

export default function RootProvider({ children }: { children: React.ReactNode }) {
  return (
    <I18nProvider>
      <AuthProvider>
        {children}
      </AuthProvider>
    </I18nProvider>
  )
}
