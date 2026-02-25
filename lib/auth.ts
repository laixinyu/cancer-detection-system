import { NextAuthOptions } from 'next-auth'
import CredentialsProvider from 'next-auth/providers/credentials'
import { buildBackendApiUrl } from './backend-api'

const insecureNextAuthSecret =
  !process.env.NEXTAUTH_SECRET ||
  process.env.NEXTAUTH_SECRET === 'your-secret-key-change-in-production'
const isLocalNextAuthUrl = (process.env.NEXTAUTH_URL ?? '').includes('localhost')
const isProductionDeployment = process.env.NODE_ENV === 'production' && !isLocalNextAuthUrl

if (isProductionDeployment && insecureNextAuthSecret) {
  throw new Error('NEXTAUTH_SECRET must be set to a strong non-default value in production')
}

export const authOptions: NextAuthOptions = {
  session: {
    strategy: 'jwt',
  },
  pages: {
    signIn: '/login',
  },
  providers: [
    CredentialsProvider({
      name: 'credentials',
      credentials: {
        email: { label: 'Email', type: 'email' },
        password: { label: 'Password', type: 'password' },
      },
      async authorize(credentials) {
        if (!credentials?.email || !credentials?.password) {
          throw new Error('Invalid credentials')
        }

        const response = await fetch(buildBackendApiUrl('/api/v1/auth/login'), {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            email: credentials.email,
            password: credentials.password,
          }),
        })
        if (!response.ok) {
          throw new Error('Invalid credentials')
        }

        const data = (await response.json()) as {
          token?: string
          user?: {
            id: string
            email: string
            name: string
            role: string
          }
        }
        if (!data.user || !data.token) {
          throw new Error('Invalid login response')
        }

        return {
          id: data.user.id,
          email: data.user.email,
          name: data.user.name,
          role: data.user.role,
          accessToken: data.token,
        }
      },
    }),
  ],
  callbacks: {
    async jwt({ token, user }) {
      if (user) {
        token.id = user.id
        token.role = user.role
        token.accessToken = user.accessToken
      }
      return token
    },
    async session({ session, token }) {
      if (token && session.user) {
        session.user.id = token.id as string
        session.user.role = token.role as string
        session.user.accessToken = token.accessToken as string
      }
      return session
    },
  },
}
