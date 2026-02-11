-- CreateEnum
CREATE TYPE "RegulatoryStatus" AS ENUM ('NOT_SUBMITTED', 'SUBMITTED', 'APPROVED', 'REJECTED');

-- CreateEnum
CREATE TYPE "DeploymentStage" AS ENUM ('RESEARCH_ONLY', 'PILOT_DECISION_SUPPORT', 'CLINICAL_DECISION_SUPPORT');

-- CreateEnum
CREATE TYPE "OpsIncidentSource" AS ENUM ('AI_SERVICE', 'APP', 'DATABASE', 'PIPELINE', 'SECURITY', 'OTHER');

-- CreateEnum
CREATE TYPE "OpsIncidentSeverity" AS ENUM ('P0', 'P1', 'P2', 'P3');

-- CreateEnum
CREATE TYPE "OpsIncidentStatus" AS ENUM ('OPEN', 'ACKNOWLEDGED', 'RESOLVED');

-- CreateTable
CREATE TABLE "clinical_evidence_runs" (
    "id" TEXT NOT NULL,
    "run_name" TEXT NOT NULL,
    "model_version" TEXT NOT NULL,
    "dataset_name" TEXT NOT NULL,
    "dataset_version" TEXT,
    "sample_count" INTEGER NOT NULL,
    "positive_count" INTEGER NOT NULL,
    "site_count" INTEGER NOT NULL,
    "auroc" DOUBLE PRECISION NOT NULL,
    "sensitivity" DOUBLE PRECISION NOT NULL,
    "specificity" DOUBLE PRECISION NOT NULL,
    "ppv" DOUBLE PRECISION,
    "npv" DOUBLE PRECISION,
    "ece" DOUBLE PRECISION,
    "brier" DOUBLE PRECISION,
    "calibration_temperature" DOUBLE PRECISION,
    "threshold_high_sensitivity" DOUBLE PRECISION,
    "threshold_high_specificity" DOUBLE PRECISION,
    "regulatory_status" "RegulatoryStatus" NOT NULL DEFAULT 'NOT_SUBMITTED',
    "stage_recommendation" "DeploymentStage" NOT NULL DEFAULT 'RESEARCH_ONLY',
    "qa_approved_by" TEXT,
    "medical_approved_by" TEXT,
    "report_path" TEXT,
    "notes" TEXT,
    "created_by_user_id" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "clinical_evidence_runs_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "ops_incidents" (
    "id" TEXT NOT NULL,
    "source" "OpsIncidentSource" NOT NULL,
    "severity" "OpsIncidentSeverity" NOT NULL,
    "status" "OpsIncidentStatus" NOT NULL DEFAULT 'OPEN',
    "title" TEXT NOT NULL,
    "detail" TEXT,
    "owner_user_id" TEXT,
    "opened_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "acknowledged_at" TIMESTAMP(3),
    "resolved_at" TIMESTAMP(3),
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "ops_incidents_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "clinical_evidence_runs_created_at_idx" ON "clinical_evidence_runs"("created_at" DESC);

-- CreateIndex
CREATE INDEX "clinical_evidence_runs_model_version_created_at_idx" ON "clinical_evidence_runs"("model_version", "created_at" DESC);

-- CreateIndex
CREATE INDEX "clinical_evidence_runs_stage_recommendation_created_at_idx" ON "clinical_evidence_runs"("stage_recommendation", "created_at" DESC);

-- CreateIndex
CREATE INDEX "ops_incidents_status_severity_opened_at_idx" ON "ops_incidents"("status", "severity", "opened_at" DESC);

-- CreateIndex
CREATE INDEX "ops_incidents_source_opened_at_idx" ON "ops_incidents"("source", "opened_at" DESC);

-- CreateIndex
CREATE INDEX "ops_incidents_owner_user_id_idx" ON "ops_incidents"("owner_user_id");

-- AddForeignKey
ALTER TABLE "clinical_evidence_runs" ADD CONSTRAINT "clinical_evidence_runs_created_by_user_id_fkey" FOREIGN KEY ("created_by_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "ops_incidents" ADD CONSTRAINT "ops_incidents_owner_user_id_fkey" FOREIGN KEY ("owner_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;
