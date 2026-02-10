'use client'

import AuthProvider from '@/components/auth-provider'
import { I18nProvider } from '@/components/i18n-provider'
import { TRPCProvider } from '@/lib/trpc'

export default function RootProvider({ children }: { children: React.ReactNode }) {
  return (
    <I18nProvider>
      <AuthProvider>
        <TRPCProvider>{children}</TRPCProvider>
      </AuthProvider>
    </I18nProvider>
  )
}
