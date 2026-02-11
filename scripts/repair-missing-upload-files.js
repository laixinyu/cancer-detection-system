#!/usr/bin/env node
/* eslint-disable @typescript-eslint/no-require-imports */
const fs = require('fs')
const path = require('path')
const { PrismaClient } = require('@prisma/client')

const prisma = new PrismaClient()

function parseArgs(argv) {
  const set = new Set(argv.slice(2))
  return {
    apply: set.has('--apply'),
    limit: (() => {
      const raw = argv.find((x) => x.startsWith('--limit='))
      if (!raw) return null
      const n = Number.parseInt(raw.slice(8), 10)
      return Number.isFinite(n) && n > 0 ? n : null
    })(),
  }
}

function toAbsPublic(filePath) {
  const rel = String(filePath || '').replace(/^\/+/, '')
  return path.join(process.cwd(), 'public', rel)
}

async function main() {
  const { apply, limit } = parseArgs(process.argv)
  const rows = await prisma.image.findMany({
    orderBy: { createdAt: 'desc' },
    ...(limit ? { take: limit } : {}),
    select: {
      id: true,
      createdAt: true,
      filePath: true,
      originalName: true,
      status: true,
    },
  })

  const existsRows = rows.filter((r) => fs.existsSync(toAbsPublic(r.filePath)))
  const missingRows = rows.filter((r) => !fs.existsSync(toAbsPublic(r.filePath)))

  const byOriginalName = new Map()
  for (const r of existsRows) {
    const list = byOriginalName.get(r.originalName) ?? []
    list.push(r)
    byOriginalName.set(r.originalName, list)
  }

  let fixed = 0
  let unresolved = 0

  for (const miss of missingRows) {
    const candidates = byOriginalName.get(miss.originalName) ?? []
    if (!candidates.length) {
      unresolved += 1
      console.log(`[unresolved] id=${miss.id} original=${miss.originalName} target=${miss.filePath}`)
      continue
    }

    // Pick nearest by createdAt.
    const targetTime = new Date(miss.createdAt).getTime()
    candidates.sort(
      (a, b) =>
        Math.abs(new Date(a.createdAt).getTime() - targetTime) -
        Math.abs(new Date(b.createdAt).getTime() - targetTime)
    )
    const source = candidates[0]
    const srcAbs = toAbsPublic(source.filePath)
    const dstAbs = toAbsPublic(miss.filePath)

    console.log(
      `[candidate] missing=${miss.id} original=${miss.originalName} source=${source.id} ${source.filePath} -> ${miss.filePath}`
    )

    if (apply) {
      fs.mkdirSync(path.dirname(dstAbs), { recursive: true })
      fs.copyFileSync(srcAbs, dstAbs)
      fixed += 1
    }
  }

  console.log('\nSummary:')
  console.log(`- mode: ${apply ? 'APPLY' : 'DRY_RUN'}`)
  console.log(`- scanned: ${rows.length}`)
  console.log(`- missing: ${missingRows.length}`)
  console.log(`- fixed: ${fixed}`)
  console.log(`- unresolved: ${unresolved}`)
}

main()
  .catch((e) => {
    console.error(e)
    process.exitCode = 1
  })
  .finally(async () => {
    await prisma.$disconnect()
  })
