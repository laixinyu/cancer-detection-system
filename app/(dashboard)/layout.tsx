'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import LanguageSwitcher from '@/components/language-switcher'
import { useI18n } from '@/components/i18n-provider'
import { useAuth } from '@/components/auth-provider'

interface NavItem {
  labelKey: string
  href: string
  icon: string
  roles: string[]
}

const navItems: NavItem[] = [
  { labelKey: 'nav.dashboard', href: '/dashboard', icon: '📊', roles: ['ADMIN', 'DOCTOR', 'PATIENT'] },
  { labelKey: 'nav.images', href: '/dashboard/images', icon: '🖼️', roles: ['PATIENT'] },
  { labelKey: 'nav.upload', href: '/dashboard/upload', icon: '⬆️', roles: ['PATIENT'] },
  { labelKey: 'nav.reviewQueue', href: '/dashboard/doctor/queue', icon: '🔍', roles: ['DOCTOR'] },
  { labelKey: 'nav.annotations', href: '/dashboard/doctor/annotations', icon: '✏️', roles: ['DOCTOR'] },
  { labelKey: 'nav.reports', href: '/dashboard/reports', icon: '📄', roles: ['DOCTOR', 'PATIENT'] },
  { labelKey: 'nav.admin', href: '/dashboard/admin', icon: '⚙️', roles: ['ADMIN'] },
]

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode
}) {
  const pathname = usePathname()
  const router = useRouter()
  const { user, logout } = useAuth()
  const { t } = useI18n()
  const userRole = user?.role || 'PATIENT'

  const filteredNavItems = navItems.filter(item => 
    item.roles.includes(userRole)
  )

  const handleSignOut = async () => {
    logout()
    router.push('/login')
  }

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Top Navigation Bar */}
      <header className="bg-white border-b border-gray-200 sticky top-0 z-50">
        <div className="container mx-auto px-4 h-16 flex items-center justify-between">
          <div className="flex items-center gap-8">
            <Link href="/dashboard" className="text-xl font-bold text-blue-600">
              🏥 {t('brand.name')}
            </Link>
            <nav className="hidden md:flex gap-6">
              {filteredNavItems.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    'flex items-center gap-2 px-3 py-2 rounded-md text-sm font-medium transition-colors',
                    pathname === item.href
                      ? 'bg-blue-100 text-blue-700'
                      : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900'
                  )}
                >
                  <span>{item.icon}</span>
                  <span>{t(item.labelKey)}</span>
                </Link>
              ))}
            </nav>
          </div>
          
          <div className="flex items-center gap-4">
            <LanguageSwitcher compact />
            <div className="text-sm text-gray-600">
              <span className="font-medium">{user?.name}</span>
              <span className="ml-2 text-xs bg-blue-100 text-blue-700 px-2 py-1 rounded">
                {userRole}
              </span>
            </div>
            <Button variant="outline" size="sm" onClick={handleSignOut}>
              {t('common.logout')}
            </Button>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="container mx-auto px-4 py-8">
        {children}
      </main>
    </div>
  )
}
