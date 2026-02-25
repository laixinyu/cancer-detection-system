package main

// File: cmd/server/orm_aliases.go
// Purpose: Gateway handlers, middleware, and wiring for external HTTP APIs.

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
