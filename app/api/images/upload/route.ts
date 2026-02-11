import { NextResponse } from 'next/server'
import { mkdir, writeFile } from 'fs/promises'
import { join } from 'path'
import { randomUUID } from 'crypto'
import { getServerSession } from 'next-auth'
import { Prisma, type FileType } from '@prisma/client'
import { authOptions } from '@/lib/auth'
import { prisma } from '@/lib/prisma'
import { CLINICAL_SCOPE } from '@/server/compliance/workflow'
import { writeAuditLog } from '@/server/compliance/audit'
import { requestAiDetection } from '@/lib/ai-service'
import { buildScreeningSummary } from '@/lib/screening'

export async function POST(request: Request) {
  try {
    const session = await getServerSession(authOptions)
    
    if (!session || !session.user) {
      return NextResponse.json({ error: 'Unauthorized' }, { status: 401 })
    }

    const formData = await request.formData()
    const file = formData.get('file') as File
    const consentAcceptedRaw = formData.get('consentAccepted')
    const consentVersion = (formData.get('consentVersion') as string) || 'v1.0'
    const consentAccepted = consentAcceptedRaw === 'true'
    
    if (!file) {
      return NextResponse.json({ error: 'No file provided' }, { status: 400 })
    }

    // Validate file type
    const allowedTypes = ['image/png', 'image/jpeg', 'image/jpg', 'image/tiff', 'application/dicom']
    if (!allowedTypes.includes(file.type)) {
      return NextResponse.json(
        { error: 'Invalid file type. Only PNG, JPEG, JPG, and TIFF are allowed' },
        { status: 400 }
      )
    }

    // Validate file size (10MB max)
    const maxSize = 10 * 1024 * 1024
    if (file.size > maxSize) {
      return NextResponse.json(
        { error: 'File too large. Maximum size is 10MB' },
        { status: 400 }
      )
    }

    // Get patient record
    let patient = await prisma.patient.findUnique({
      where: { userId: session.user.id },
    })

    if (!patient && session.user.role === 'PATIENT') {
      // Create patient record if it doesn't exist
      patient = await prisma.patient.create({
        data: {
          userId: session.user.id,
          dateOfBirth: new Date('1990-01-01'), // Default, should be updated
          gender: 'OTHER',
        },
      })
    }

    if (!patient) {
      return NextResponse.json(
        { error: 'Patient profile not found' },
        { status: 400 }
      )
    }

    if (session.user.role === 'PATIENT' && !consentAccepted) {
      return NextResponse.json(
        {
          error: 'Consent required before upload',
          clinicalScope: CLINICAL_SCOPE,
        },
        { status: 400 }
      )
    }

    if (session.user.role === 'PATIENT' && consentAccepted) {
      await prisma.patientConsent.upsert({
        where: {
          patientId_consentType_consentVersion: {
            patientId: patient.id,
            consentType: 'AI_ANALYSIS',
            consentVersion,
          },
        },
        create: {
          patientId: patient.id,
          consentType: 'AI_ANALYSIS',
          consentVersion,
          accepted: true,
          acceptedByUserId: session.user.id,
        },
        update: {
          accepted: true,
          acceptedAt: new Date(),
          acceptedByUserId: session.user.id,
        },
      })
    }

    // Save file
    const bytes = await file.arrayBuffer()
    const buffer = Buffer.from(bytes)
    
    // Create uploads directory if it doesn't exist
    const uploadsDir = join(process.cwd(), 'public', 'uploads')
    const rawExt = file.name.split('.').pop()?.toLowerCase() || ''
    const allowedExtensions = new Set(['png', 'jpg', 'jpeg', 'tiff', 'dcm'])
    const normalizedExt = allowedExtensions.has(rawExt)
      ? rawExt
      : file.type === 'application/dicom'
      ? 'dcm'
      : 'jpg'
    const fileName = `${Date.now()}_${randomUUID()}.${normalizedExt}`
    const filePath = join(uploadsDir, fileName)

    await mkdir(uploadsDir, { recursive: true })
    await writeFile(filePath, buffer)

    // Get file extension
    const fileExtension = file.name.split('.').pop()?.toUpperCase() || 'JPEG'
    const fileTypeMap: Record<string, FileType> = {
      'PNG': 'PNG',
      'JPG': 'JPEG',
      'JPEG': 'JPEG',
      'TIFF': 'TIFF',
      'DCM': 'DICOM',
    }
    
    const fileType = fileTypeMap[fileExtension] || 'JPEG'

    // Create image record
    const image = await prisma.image.create({
      data: {
        patientId: patient.id,
        filePath: `/uploads/${fileName}`,
        fileType: fileType,
        originalName: file.name,
        fileSize: file.size,
        status: 'PROCESSING',
        uploadedBy: session.user.id,
      },
    })

    try {
      const aiResult = await requestAiDetection(file)
      const screeningSummary = buildScreeningSummary(aiResult)
      const triage =
        screeningSummary.triagePriority === 'CRITICAL' || screeningSummary.triagePriority === 'HIGH'
          ? 'HIGH_RISK'
          : aiResult.cancerProbability >= CLINICAL_SCOPE.highRiskThreshold
          ? 'HIGH_RISK'
          : 'ROUTINE'

      await prisma.detection.create({
        data: {
          imageId: image.id,
          modelVersion: aiResult.modelVersion,
          cancerProbability: aiResult.cancerProbability,
          findings: {
            triage,
            regions: aiResult.regions,
            labelScores: aiResult.labelScores ?? {},
            topFindings: aiResult.topFindings ?? [],
            decisionHighSensitivity: aiResult.decisionHighSensitivity ?? false,
            decisionHighSpecificity: aiResult.decisionHighSpecificity ?? false,
            taskDecisions: aiResult.taskDecisions ?? {},
            operatingPointsUsed: aiResult.operatingPointsUsed ?? {},
            calibrationTemperature: aiResult.calibrationTemperature ?? null,
            clinicalUse: aiResult.clinicalUse ?? 'RESEARCH_ONLY',
            clinicalStage: aiResult.clinicalStage ?? aiResult.clinicalUse ?? 'RESEARCH_ONLY',
            detectorModelLoaded: aiResult.detectorModelLoaded ?? false,
            screeningSummary,
          } as unknown as Prisma.InputJsonValue,
          heatmapPath: aiResult.heatmapPath,
          status: 'PENDING',
        },
      })

      await prisma.image.update({
        where: { id: image.id },
        data: { status: 'COMPLETED' },
      })
    } catch (error) {
      console.error('AI detection error:', error)
      await prisma.image.update({
        where: { id: image.id },
        data: { status: 'FAILED' },
      })
      await writeAuditLog(prisma, {
        actorUserId: session.user.id,
        actorRole: session.user.role,
        action: 'AI_DETECTION_FAILED',
        entityType: 'image',
        entityId: image.id,
        result: 'FAILED',
        metadata: {
          reason: error instanceof Error ? error.message : 'Unknown AI error',
        },
      })
    }

    await writeAuditLog(prisma, {
      actorUserId: session.user.id,
      actorRole: session.user.role,
      action: 'IMAGE_UPLOAD',
      entityType: 'image',
      entityId: image.id,
      result: 'SUCCESS',
      metadata: {
        fileType,
        fileSize: file.size,
      },
    })

    const latestImage = await prisma.image.findUnique({
      where: { id: image.id },
      select: {
        id: true,
        originalName: true,
        filePath: true,
        status: true,
      },
    })

    return NextResponse.json({
      success: true,
      image: {
        id: latestImage?.id ?? image.id,
        originalName: latestImage?.originalName ?? image.originalName,
        filePath: latestImage?.filePath ?? image.filePath,
        status: latestImage?.status ?? image.status,
      },
    })
  } catch (error) {
    console.error('Upload error:', error)
    return NextResponse.json(
      { error: 'Upload failed' },
      { status: 500 }
    )
  }
}
