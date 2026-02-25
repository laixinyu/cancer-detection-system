import { mkdir, readFile, writeFile } from 'fs/promises'
import { extname, join } from 'path'
import { randomUUID } from 'crypto'
import {
  S3Client,
  PutObjectCommand,
  GetObjectCommand,
  HeadBucketCommand,
  CreateBucketCommand,
} from '@aws-sdk/client-s3'

type StorageProvider = 'local' | 's3' | 'fs'

export interface SaveUploadInput {
  buffer: Buffer
  originalName: string
  contentType?: string
}

export interface StoredBlob {
  data: Buffer
  contentType: string
}

const ensuredBuckets = new Set<string>()

function normalizeStorageEnv(value: string): string {
  const v = value.trim().toLowerCase()
  if (v === 'production' || v === 'prod') return 'prod'
  if (v === 'development' || v === 'dev' || v === 'local') return 'dev'
  if (v === 'test' || v === 'testing' || v === 'ci') return 'test'
  return v.replace(/[^a-z0-9-]/g, '-')
}

function getStorageEnvTag(): string {
  const raw = process.env.STORAGE_ENV || process.env.NODE_ENV || 'development'
  const normalized = normalizeStorageEnv(raw)
  return normalized || 'dev'
}

function getStorageProvider(): StorageProvider {
  const provider = (process.env.STORAGE_PROVIDER || 'local').toLowerCase()
  if (provider === 'fs' || provider === 'filesystem') {
    return 'fs'
  }
  return provider === 's3' ? 's3' : 'local'
}

function normalizeExtension(name: string): string {
  const ext = extname(name).toLowerCase()
  const safe = new Set(['.png', '.jpg', '.jpeg', '.tiff', '.dcm'])
  return safe.has(ext) ? ext : '.jpg'
}

function buildFilename(originalName: string): string {
  return `${Date.now()}_${randomUUID()}${normalizeExtension(originalName)}`
}

function getLocalConfig() {
  const envTag = getStorageEnvTag()
  const useEnvSubdir = (process.env.LOCAL_UPLOAD_ENV_SUBDIR || 'true').toLowerCase() === 'true'
  const rootDir = process.env.LOCAL_UPLOAD_DIR || join(process.cwd(), 'public', 'uploads')
  const baseDir = useEnvSubdir ? join(rootDir, envTag) : rootDir
  const publicPrefix = (process.env.LOCAL_UPLOAD_PUBLIC_PREFIX || '/uploads').replace(/\/$/, '')
  return { baseDir, publicPrefix }
}

function parseS3Uri(uri: string): { bucket: string; key: string } | null {
  if (!uri.startsWith('s3://')) {
    return null
  }
  const raw = uri.slice(5)
  const slash = raw.indexOf('/')
  if (slash <= 0) {
    return null
  }
  const bucket = raw.slice(0, slash)
  const key = raw.slice(slash + 1)
  if (!bucket || !key) {
    return null
  }
  return { bucket, key }
}

function createS3Client(provider: StorageProvider): S3Client {
  const isLocalObject = provider === 'local'
  const endpoint = process.env.S3_ENDPOINT || (isLocalObject ? 'http://127.0.0.1:9000' : undefined)
  const region = process.env.S3_REGION || 'us-east-1'
  const accessKeyId = process.env.S3_ACCESS_KEY_ID || (isLocalObject ? 'minioadmin' : undefined)
  const secretAccessKey = process.env.S3_SECRET_ACCESS_KEY || (isLocalObject ? 'minioadmin' : undefined)
  const forcePathStyle = (process.env.S3_FORCE_PATH_STYLE || 'true').toLowerCase() === 'true'

  if (!accessKeyId || !secretAccessKey) {
    throw new Error('S3_ACCESS_KEY_ID/S3_SECRET_ACCESS_KEY is required for object storage')
  }

  return new S3Client({
    region,
    endpoint,
    forcePathStyle,
    credentials: {
      accessKeyId,
      secretAccessKey,
    },
  })
}

function getEnvSpecific(name: string, envTag: string): string {
  const key = `${name}_${envTag.toUpperCase()}`
  return (process.env[key] || '').trim()
}

function resolveS3Target(provider: StorageProvider): { bucket: string; keyPrefix: string } {
  const envTag = getStorageEnvTag()
  const baseBucket =
    getEnvSpecific('S3_BUCKET', envTag) ||
    (process.env.S3_BUCKET || '').trim() ||
    (provider === 'local' ? `cancer-images-${envTag}` : '')
  if (!baseBucket) {
    throw new Error('S3_BUCKET is required for object storage')
  }

  const basePrefix =
    getEnvSpecific('S3_PREFIX', envTag) ||
    (process.env.S3_PREFIX || 'uploads').trim()
  const normalizedBasePrefix = basePrefix.replace(/^\/+|\/+$/g, '')
  const includeEnvInPrefix = (process.env.STORAGE_ENV_IN_PREFIX || 'true').toLowerCase() === 'true'
  const keyPrefix = includeEnvInPrefix ? `${envTag}/${normalizedBasePrefix}` : normalizedBasePrefix
  return { bucket: baseBucket, keyPrefix }
}

function guessContentTypeFromPath(path: string): string {
  const ext = extname(path).toLowerCase()
  if (ext === '.png') return 'image/png'
  if (ext === '.jpg' || ext === '.jpeg') return 'image/jpeg'
  if (ext === '.tiff' || ext === '.tif') return 'image/tiff'
  if (ext === '.dcm') return 'application/dicom'
  return 'application/octet-stream'
}

export async function saveUploadedFile(input: SaveUploadInput): Promise<string> {
  const provider = getStorageProvider()
  const fileName = buildFilename(input.originalName)

  if (provider === 'fs') {
    const { baseDir, publicPrefix } = getLocalConfig()
    await mkdir(baseDir, { recursive: true })
    const absolutePath = join(baseDir, fileName)
    await writeFile(absolutePath, input.buffer)
    return `${publicPrefix}/${fileName}`
  }

  const target = resolveS3Target(provider)
  const bucket = target.bucket
  const key = `${target.keyPrefix}/${fileName}`
  const client = createS3Client(provider)
  if (!ensuredBuckets.has(bucket)) {
    try {
      await client.send(new HeadBucketCommand({ Bucket: bucket }))
    } catch {
      await client.send(new CreateBucketCommand({ Bucket: bucket }))
    }
    ensuredBuckets.add(bucket)
  }
  await client.send(
    new PutObjectCommand({
      Bucket: bucket,
      Key: key,
      Body: input.buffer,
      ContentType: input.contentType || guessContentTypeFromPath(fileName),
    })
  )
  return `s3://${bucket}/${key}`
}

export function buildImageAccessUrl(imageId: string, storedPath: string): string {
  if (storedPath.startsWith('s3://')) {
    return `/api/images/${imageId}/file`
  }
  return storedPath
}

export async function readStoredFile(storedPath: string): Promise<StoredBlob> {
  if (storedPath.startsWith('s3://')) {
    const ref = parseS3Uri(storedPath)
    if (!ref) {
      throw new Error(`Invalid S3 URI: ${storedPath}`)
    }
    const client = createS3Client(getStorageProvider())
    const output = await client.send(
      new GetObjectCommand({
        Bucket: ref.bucket,
        Key: ref.key,
      })
    )
    if (!output.Body) {
      throw new Error('S3 object body is empty')
    }
    const contentType = output.ContentType || guessContentTypeFromPath(ref.key)
    const bytes = await output.Body.transformToByteArray()
    return { data: Buffer.from(bytes), contentType }
  }

  const { baseDir } = getLocalConfig()
  let absolutePath = storedPath
  const publicPrefix = (process.env.LOCAL_UPLOAD_PUBLIC_PREFIX || '/uploads').replace(/\/$/, '')
  if (storedPath.startsWith(`${publicPrefix}/`)) {
    absolutePath = join(baseDir, storedPath.slice(publicPrefix.length + 1))
  }
  const data = await readFile(absolutePath)
  return { data, contentType: guessContentTypeFromPath(storedPath) }
}
