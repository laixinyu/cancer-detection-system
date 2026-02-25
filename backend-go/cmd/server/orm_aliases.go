package main

// 文件： cmd/server/orm_aliases.go
// 用途：网关的处理器、中间件与对外 HTTP API 路由装配。

import "cancer-detection-backend/internal/domain"

type ormUser = domain.User
type ormPatient = domain.Patient
type ormImage = domain.Image
type ormDetection = domain.Detection
type ormReport = domain.Report
type ormAuditLog = domain.AuditLog
type ormClinicalEvidenceRun = domain.ClinicalEvidenceRun
type ormOpsIncident = domain.OpsIncident
type ormPatientConsent = domain.PatientConsent
