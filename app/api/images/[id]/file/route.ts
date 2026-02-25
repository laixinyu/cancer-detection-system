import { NextResponse } from 'next/server'
import { getServerSession } from 'next-auth'
import { authOptions } from '@/lib/auth'
import { prisma } from '@/lib/prisma'
import { readStoredFile } from '@/lib/storage'

export async function GET(
  _request: Request,
  context: { params: { id: string } }
) {
  const session = await getServerSession(authOptions)
  if (!session?.user) {
    return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
  }

  const { id } = context.params
  const image = await prisma.image.findUnique({
    where: { id },
    select: {
      id: true,
      filePath: true,
      originalName: true,
      uploadedBy: true,
    },
  })
  if (!image) {
    return NextResponse.json({ error: 'Image not found' }, { status: 404 })
  }

  if (session.user.role === 'PATIENT' && image.uploadedBy !== session.user.id) {
    return NextResponse.json({ error: 'Forbidden' }, { status: 403 })
  }

  try {
    const blob = await readStoredFile(image.filePath)
    return new NextResponse(blob.data, {
      status: 200,
      headers: {
        'Content-Type': blob.contentType,
        'Cache-Control': 'private, max-age=60',
        'Content-Disposition': `inline; filename="${encodeURIComponent(image.originalName)}"`,
      },
    })
  } catch (error) {
    console.error('Read image file error:', error)
    return NextResponse.json({ error: 'File unavailable' }, { status: 502 })
  }
}
