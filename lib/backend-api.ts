export function getBackendApiBaseUrl(): string {
  const serverBase =
    process.env.BACKEND_API_URL ||
    process.env.NEXT_PUBLIC_BACKEND_API_URL ||
    'http://localhost:8080'

  if (typeof window === 'undefined') {
    return serverBase.replace(/\/$/, '')
  }

  const clientBase = process.env.NEXT_PUBLIC_BACKEND_API_URL || 'http://localhost:8080'
  return clientBase.replace(/\/$/, '')
}

export function buildBackendApiUrl(path: string): string {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  return `${getBackendApiBaseUrl()}${normalizedPath}`
}
