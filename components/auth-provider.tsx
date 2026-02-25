'use client'

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { buildBackendApiUrl } from '@/lib/backend-api'

type AuthUser = {
  id: string
  email: string
  name: string
  role: string
}

type AuthStatus = 'loading' | 'authenticated' | 'unauthenticated'

type AuthContextValue = {
  user: AuthUser | null
  token: string | null
  status: AuthStatus
  login: (email: string, password: string) => Promise<void>
  logout: () => void
  authFetch: (path: string, init?: RequestInit) => Promise<Response>
}

const AUTH_STORAGE_KEY = 'cds_auth_v1'

const AuthContext = createContext<AuthContextValue | null>(null)

function readAuthStateFromStorage() {
  if (typeof window === 'undefined') {
    return { token: null, user: null, status: 'loading' as const }
  }
  const raw = localStorage.getItem(AUTH_STORAGE_KEY)
  if (!raw) {
    return { token: null, user: null, status: 'unauthenticated' as const }
  }
  try {
    const parsed = JSON.parse(raw) as { token?: string }
    if (!parsed.token) {
      return { token: null, user: null, status: 'unauthenticated' as const }
    }
    const parsedUser = getUserFromToken(parsed.token)
    if (!parsedUser) {
      localStorage.removeItem(AUTH_STORAGE_KEY)
      return { token: null, user: null, status: 'unauthenticated' as const }
    }
    return { token: parsed.token, user: parsedUser, status: 'authenticated' as const }
  } catch {
    localStorage.removeItem(AUTH_STORAGE_KEY)
    return { token: null, user: null, status: 'unauthenticated' as const }
  }
}

function parseJwtPayload(token: string): Record<string, unknown> | null {
  try {
    const part = token.split('.')[1]
    if (!part) return null
    const normalized = part.replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4)
    const decoded = atob(padded)
    return JSON.parse(decoded) as Record<string, unknown>
  } catch {
    return null
  }
}

function getUserFromToken(token: string): AuthUser | null {
  const payload = parseJwtPayload(token)
  if (!payload) return null
  const id = typeof payload.uid === 'string' ? payload.uid : typeof payload.sub === 'string' ? payload.sub : ''
  const email = typeof payload.email === 'string' ? payload.email : ''
  const name = typeof payload.name === 'string' ? payload.name : ''
  const role = typeof payload.role === 'string' ? payload.role : ''
  if (!id || !email || !name || !role) return null
  return { id, email, name, role }
}

export default function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<{ token: string | null; user: AuthUser | null; status: AuthStatus }>(() => readAuthStateFromStorage())

  useEffect(() => {
    if (state.status !== 'loading') return
    const timer = setTimeout(() => {
      setState(readAuthStateFromStorage())
    }, 0)
    return () => clearTimeout(timer)
  }, [state.status])

  const login = useCallback(async (email: string, password: string) => {
    const response = await fetch(buildBackendApiUrl('/api/v1/auth/login'), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ email, password }),
    })
    const data = await response.json().catch(() => ({}))
    if (!response.ok || !data.token) {
      const message =
        data && typeof data.error === 'string' ? data.error : 'Invalid credentials'
      throw new Error(message)
    }
    const parsedUser = getUserFromToken(data.token as string)
    if (!parsedUser) {
      throw new Error('Invalid login token')
    }
    localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify({ token: data.token }))
    setState({ token: data.token as string, user: parsedUser, status: 'authenticated' })
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem(AUTH_STORAGE_KEY)
    setState({ token: null, user: null, status: 'unauthenticated' })
  }, [])

  const authFetch = useCallback(
    (path: string, init?: RequestInit) => {
      if (!state.token) {
        return Promise.resolve(
          new Response(JSON.stringify({ error: 'Unauthorized' }), {
            status: 401,
            headers: { 'Content-Type': 'application/json' },
          })
        )
      }
      return fetch(buildBackendApiUrl(path), {
        ...init,
        headers: {
          ...(init?.headers || {}),
          Authorization: `Bearer ${state.token}`,
        },
      })
    },
    [state.token]
  )

  const { user, token, status } = state

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      token,
      status,
      login,
      logout,
      authFetch,
    }),
    [user, token, status, login, logout, authFetch]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider')
  }
  return context
}
