import { createTRPCRouter } from './trpc'
import { userRouter } from './routers/user'
import { imageRouter } from './routers/image'
import { detectionRouter } from './routers/detection'
import { reportRouter } from './routers/report'
import { complianceRouter } from './routers/compliance'
import { auditRouter } from './routers/audit'
import { analyticsRouter } from './routers/analytics'
import { opsRouter } from './routers/ops'

export const appRouter = createTRPCRouter({
  user: userRouter,
  image: imageRouter,
  detection: detectionRouter,
  report: reportRouter,
  compliance: complianceRouter,
  audit: auditRouter,
  analytics: analyticsRouter,
  ops: opsRouter,
})

export type AppRouter = typeof appRouter
