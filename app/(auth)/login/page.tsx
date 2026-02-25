'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import LanguageSwitcher from '@/components/language-switcher'
import { useI18n } from '@/components/i18n-provider'
import { useAuth } from '@/components/auth-provider'

export default function LoginPage() {
  const router = useRouter()
  const { t } = useI18n()
  const { login } = useAuth()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setIsLoading(true)

    try {
      await login(email, password)
      router.push('/dashboard')
      router.refresh()
    } catch {
      setError(t('auth.login.error'))
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-md space-y-4">
        <div className="flex justify-end">
          <LanguageSwitcher compact />
        </div>
      <Card className="w-full">
        <CardHeader>
          <CardTitle className="text-center">{t('auth.login.title')}</CardTitle>
          <p className="text-center text-gray-600 mt-2">{t('auth.login.subtitle')}</p>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <div className="bg-red-50 text-red-600 p-3 rounded-md text-sm">
                {error}
              </div>
            )}
            
            <div>
              <label htmlFor="email" className="block text-sm font-medium mb-2">
                {t('auth.login.email')}
              </label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                placeholder="doctor@example.com"
              />
            </div>

            <div>
              <label htmlFor="password" className="block text-sm font-medium mb-2">
                {t('auth.login.password')}
              </label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                placeholder="••••••••"
              />
            </div>

            <Button type="submit" className="w-full" disabled={isLoading}>
              {isLoading ? t('auth.login.submitting') : t('auth.login.submit')}
            </Button>

            <div className="text-center text-sm">
              <span className="text-gray-600">{t('auth.login.noAccount')} </span>
              <Link href="/register" className="text-blue-600 hover:underline">
                {t('auth.login.register')}
              </Link>
            </div>
          </form>
        </CardContent>
      </Card>
      </div>
    </div>
  )
}
