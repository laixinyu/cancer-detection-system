import 'next-auth'
import 'next-auth/jwt'

declare module 'next-auth' {
  interface Session {
    user: {
      id: string
      email: string
      name: string
      role: string
      accessToken: string
    }
  }

  interface User {
    id: string
    email: string
    name: string
    role: string
    accessToken: string
  }
}

declare module 'next-auth/jwt' {
  interface JWT {
    id: string
    role: string
    accessToken: string
  }
}
