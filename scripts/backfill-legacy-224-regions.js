#!/usr/bin/env node
/* eslint-disable @typescript-eslint/no-require-imports */
const path = require('path')
const { PrismaClient } = require('@prisma/client')
const sharp = require('sharp')

const prisma = new PrismaClient()

function parseArgs(argv) {
  const args = new Set(argv.slice(2))
  return {
    apply: args.has('--apply'),
    limit: (() => {
      const raw = argv.find((v) => v.startsWith('--limit='))
      if (!raw) return null
      const n = Number.parseInt(raw.slice('--limit='.length), 10)
      return Number.isFinite(n) && n > 0 ? n : null
    })(),
  }
}

function isNumber(v) {
  return typeof v === 'number' && Number.isFinite(v)
}

function isLegacy224RegionSet(regions, imageWidth, imageHeight) {
  if (!Array.isArray(regions) || regions.length === 0) return false
  if (!(imageWidth > 256 || imageHeight > 256)) return false

  const valid = regions.every(
    (r) =>
      r &&
      typeof r === 'object' &&
      isNumber(r.x) &&
      isNumber(r.y) &&
      isNumber(r.width) &&
      isNumber(r.height)
  )
  if (!valid) return false

  const maxX = Math.max(...regions.map((r) => r.x + r.width))
  const maxY = Math.max(...regions.map((r) => r.y + r.height))
  return maxX <= 256 && maxY <= 256
}

function remapFrom224(regions, imageWidth, imageHeight) {
  const sx = imageWidth / 224
  const sy = imageHeight / 224
  return regions.map((r) => ({
    ...r,
    x: Number((r.x * sx).toFixed(6)),
    y: Number((r.y * sy).toFixed(6)),
    width: Number((r.width * sx).toFixed(6)),
    height: Number((r.height * sy).toFixed(6)),
  }))
}

async function readImageSize(filePath) {
  const abs = path.join(process.cwd(), 'public', filePath.replace(/^\/+/, ''))
  const metadata = await sharp(abs).metadata()
  if (!metadata.width || !metadata.height) {
    throw new Error(`Unable to read image dimensions: ${abs}`)
  }
  return { width: metadata.width, height: metadata.height, absPath: abs }
}

async function main() {
  const { apply, limit } = parseArgs(process.argv)
  const detections = await prisma.detection.findMany({
    orderBy: { createdAt: 'desc' },
    ...(limit ? { take: limit } : {}),
    select: {
      id: true,
      createdAt: true,
      findings: true,
      image: {
        select: {
          filePath: true,
          originalName: true,
        },
      },
    },
  })

  let scanned = 0
  let candidates = 0
  let updated = 0
  let skippedNoImage = 0
  let skippedReadError = 0

  for (const row of detections) {
    scanned += 1
    const findings = row.findings && typeof row.findings === 'object' ? row.findings : null
    const regions = findings && Array.isArray(findings.regions) ? findings.regions : null
    if (!regions || regions.length === 0) continue

    let size
    try {
      size = await readImageSize(row.image.filePath)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      if (message.includes('Input file is missing')) {
        skippedNoImage += 1
      } else {
        skippedReadError += 1
      }
      console.warn(`[skip:image] detection=${row.id} file=${row.image.filePath} reason=${message}`)
      continue
    }

    if (!isLegacy224RegionSet(regions, size.width, size.height)) {
      continue
    }

    candidates += 1
    const newRegions = remapFrom224(regions, size.width, size.height)
    const newFindings = {
      ...findings,
      regions: newRegions,
      regionCoordinateSpace: 'original_image_pixels',
      regionBackfilledFrom: 'legacy_224_space',
      regionBackfilledAt: new Date().toISOString(),
    }

    const before = regions[0]
    const after = newRegions[0]
    console.log(
      `[candidate] detection=${row.id} image=${row.image.originalName} size=${size.width}x${size.height} ` +
        `sample=(${before.x},${before.y},${before.width},${before.height})->(${after.x},${after.y},${after.width},${after.height})`
    )

    if (apply) {
      await prisma.detection.update({
        where: { id: row.id },
        data: { findings: newFindings },
      })
      updated += 1
    }
  }

  console.log('\nSummary:')
  console.log(`- mode: ${apply ? 'APPLY' : 'DRY_RUN'}`)
  console.log(`- scanned: ${scanned}`)
  console.log(`- candidates: ${candidates}`)
  console.log(`- updated: ${updated}`)
  console.log(`- skipped_no_image: ${skippedNoImage}`)
  console.log(`- skipped_read_error: ${skippedReadError}`)
}

main()
  .catch((err) => {
    console.error(err)
    process.exitCode = 1
  })
  .finally(async () => {
    await prisma.$disconnect()
  })
