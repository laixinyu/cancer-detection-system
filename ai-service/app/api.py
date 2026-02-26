import asyncio
import logging
import os
import time
from glob import glob
from typing import Optional

import numpy as np
from fastapi import FastAPI, File, Header, HTTPException, Response, UploadFile

from app.config import AppSettings, load_settings
from app.metrics import Metrics, build_metrics, register_metrics_middleware, register_metrics_routes
from app.model_runtime import (
    LABEL_MASS,
    LABEL_NODULE,
    LABEL_OPACITY,
    LABEL_PNEUMONIA,
    ModelRunner,
    PredictResponse,
    TASK_LABELS,
    TASK_LESION,
    TASK_PNEUMONIA,
)

logger = logging.getLogger("ai-service")
logging.basicConfig(level=os.getenv("AI_LOG_LEVEL", "INFO").upper())


class ServiceContext:
    def __init__(self, settings: AppSettings, runner: ModelRunner, metrics: Metrics) -> None:
        self.settings = settings
        self.runner = runner
        self.metrics = metrics
        self.predict_semaphore = asyncio.Semaphore(settings.max_concurrent_requests)



def create_app(
    settings: Optional[AppSettings] = None,
    runner: Optional[ModelRunner] = None,
    metrics: Optional[Metrics] = None,
) -> FastAPI:
    app = FastAPI(title="Cancer Detection AI Service", version="1.1.0")
    settings = settings or load_settings()
    runner = runner or ModelRunner()
    metrics = metrics or build_metrics()

    ctx = ServiceContext(settings=settings, runner=runner, metrics=metrics)
    app.state.ctx = ctx

    register_metrics_middleware(app, metrics)
    register_metrics_routes(app)

    @app.on_event("startup")
    def on_startup():
        logger.info(
            "AI service startup: model_path=%s, max_concurrency=%s, inference_timeout=%ss, queue_wait=%ss",
            ctx.runner.model_path,
            ctx.settings.max_concurrent_requests,
            ctx.settings.inference_timeout_seconds,
            ctx.settings.max_queue_wait_seconds,
        )

    @app.get("/livez")
    def livez():
        return {"status": "alive"}

    @app.get("/health")
    def health(response: Response):
        ready = ctx.runner.ensure_ready()
        allowed, blocked_reason = ctx.runner.prediction_allowed()
        healthy = ready and allowed
        if not healthy:
            response.status_code = 503

        available_models = sorted([os.path.basename(path) for path in glob("./models/*.onnx")])
        return {
            "status": "ok" if healthy else "degraded",
            "predictionReady": healthy,
            "predictionBlockedReason": blocked_reason if not healthy else None,
            "modelLoaded": ctx.runner.session is not None,
            "modelVersion": ctx.runner.model_version,
            "modelPath": ctx.runner.model_path,
            "calibrationTemperature": ctx.runner.calibration_temperature,
            "thresholdHighSensitivity": ctx.runner.high_sensitivity_threshold,
            "thresholdHighSpecificity": ctx.runner.high_specificity_threshold,
            "taskThresholds": ctx.runner.task_thresholds,
            "clinicalConfigSource": ctx.runner.config_source,
            "clinicalStage": ctx.runner.clinical_stage,
            "governanceGatePassed": ctx.runner.governance_gate_passed,
            "governanceGateMessage": ctx.runner.governance_gate_message,
            "governanceCriteria": {
                "minSiteCount": ctx.runner.governance_min_site_count,
                "minAuroc": ctx.runner.governance_min_auroc,
                "minSensitivity": ctx.runner.governance_min_sensitivity,
                "minSpecificity": ctx.runner.governance_min_specificity,
            },
            "heuristicRegionsEnabled": ctx.runner.enable_heuristic_regions,
            "detectorModelLoaded": ctx.runner.detector_session is not None,
            "detectorModelPath": ctx.runner.detector_model_path,
            "detectorModelSha256": ctx.runner.detector_sha256,
            "detectorExpectedSha256": ctx.runner.detector_expected_sha256 or None,
            "detectorProfilePath": ctx.runner.detector_profile_path,
            "detectorDecoderFormat": ctx.runner.detector_profile.get("decoder_format", "auto"),
            "enableTTA": ctx.runner.enable_tta,
            "enableFPReduction": ctx.runner.enable_fp_reduction,
            "fpMinLungOverlap": ctx.runner.fp_min_lung_overlap,
            "detectorStartupCheckPassed": ctx.runner.detector_check_passed,
            "detectorStartupCheckMessage": ctx.runner.detector_check_message,
            "tasks": ["A:Pneumonia", "B:Nodule/Mass", "C:InfectionCoverage+WhiteLung"],
            "availableModels": available_models,
            "modelError": ctx.runner.model_error,
            "ortAvailableProviders": ctx.runner.ort_available_providers,
            "modelActiveProviders": ctx.runner.model_active_providers,
            "detectorActiveProviders": ctx.runner.detector_active_providers,
            "ortPreferCuda": ctx.runner.prefer_cuda_ep,
            "ortPreferDirectML": ctx.runner.prefer_dml_ep,
            "ortEnableIOBinding": ctx.runner.enable_io_binding,
            "ortCudaDeviceId": ctx.runner.ort_cuda_device_id,
            "ortGraphOptimization": ctx.runner.ort_graph_optimization,
            "maxConcurrentRequests": ctx.settings.max_concurrent_requests,
            "inferenceTimeoutSeconds": ctx.settings.inference_timeout_seconds,
            "maxQueueWaitSeconds": ctx.settings.max_queue_wait_seconds,
        }

    @app.post("/predict", response_model=PredictResponse)
    async def predict(file: UploadFile = File(...), x_api_key: Optional[str] = Header(default=None)):
        if ctx.settings.require_api_key:
            if not ctx.settings.api_key:
                raise HTTPException(status_code=503, detail="API key auth misconfigured")
            if x_api_key != ctx.settings.api_key:
                raise HTTPException(status_code=401, detail="Unauthorized")

        if not ctx.runner.ensure_ready():
            raise HTTPException(status_code=503, detail="Inference model unavailable")
        allowed, blocked_reason = ctx.runner.prediction_allowed()
        if not allowed:
            raise HTTPException(status_code=503, detail=blocked_reason)

        if not file.content_type:
            raise HTTPException(status_code=400, detail="Missing content type")

        allowed_types = {
            "image/png",
            "image/jpeg",
            "image/jpg",
            "image/tiff",
            "application/dicom",
            "application/octet-stream",
        }
        if file.content_type not in allowed_types:
            raise HTTPException(status_code=400, detail=f"Unsupported content type: {file.content_type}")

        content = await file.read()
        if not content:
            raise HTTPException(status_code=400, detail="Empty file")
        if len(content) > ctx.settings.max_upload_bytes:
            raise HTTPException(
                status_code=413,
                detail=f"File too large. Max allowed is {ctx.settings.max_upload_bytes} bytes",
            )

        try:
            await asyncio.wait_for(
                ctx.predict_semaphore.acquire(),
                timeout=ctx.settings.max_queue_wait_seconds,
            )
        except asyncio.TimeoutError as exc:
            raise HTTPException(status_code=429, detail="Inference queue is full, retry later") from exc

        ctx.metrics.predict_inflight.inc()
        try:
            infer_start = time.perf_counter()
            try:
                image_norm, image_original = await asyncio.wait_for(
                    asyncio.to_thread(ctx.runner.preprocess_with_original, content),
                    timeout=ctx.settings.inference_timeout_seconds,
                )
                model_scores = await asyncio.wait_for(
                    asyncio.to_thread(ctx.runner.infer_multitask_scores, image_norm),
                    timeout=ctx.settings.inference_timeout_seconds,
                )
                regions, region_scores = await asyncio.wait_for(
                    asyncio.to_thread(ctx.runner.detect_regions_with_detector, image_original, model_scores),
                    timeout=ctx.settings.inference_timeout_seconds,
                )
                if not regions and ctx.runner.enable_heuristic_regions:
                    regions, region_scores = await asyncio.wait_for(
                        asyncio.to_thread(ctx.runner.detect_regions, image_original, model_scores),
                        timeout=ctx.settings.inference_timeout_seconds,
                    )
            except asyncio.TimeoutError as exc:
                raise HTTPException(status_code=504, detail="Inference timed out") from exc
            except Exception as exc:
                logger.exception("Inference execution failed")
                raise HTTPException(status_code=500, detail="Inference execution failed") from exc
            finally:
                ctx.metrics.predict_infer_seconds.observe(time.perf_counter() - infer_start)
        finally:
            ctx.metrics.predict_inflight.dec()
            ctx.predict_semaphore.release()

        merged_scores = {
            label: float(np.clip(max(model_scores.get(label, 0.0), region_scores.get(label, 0.0)), 0.0, 1.0))
            for label in TASK_LABELS
        }

        white_lung_assessment = ctx.runner.assess_white_lung(image_original, merged_scores[LABEL_OPACITY])
        merged_scores, low_evidence_mode = ctx.runner.apply_low_evidence_guard(
            merged_scores, regions, white_lung_assessment
        )
        white_lung_assessment = ctx.runner.assess_white_lung(image_original, merged_scores[LABEL_OPACITY])

        cancer_probability = max(
            merged_scores[LABEL_NODULE],
            merged_scores[LABEL_MASS],
            merged_scores[LABEL_OPACITY],
        )

        top_findings_threshold = 0.5
        top_findings = [
            label
            for label, score in sorted(merged_scores.items(), key=lambda item: item[1], reverse=True)
            if score > top_findings_threshold
        ][:3]
        infection_coverage = ctx.runner.build_infection_coverage(merged_scores, white_lung_assessment)
        infection_top = []
        if regions:
            infection_top_threshold = 0.45 if low_evidence_mode else 0.35
            infection_top = [
                key
                for key, score in sorted(infection_coverage.items(), key=lambda item: item[1], reverse=True)
                if score >= infection_top_threshold
            ][:2]
            top_findings = top_findings + infection_top

        pneumonia_probability = merged_scores[LABEL_PNEUMONIA]
        lesion_hs = ctx.runner.task_thresholds[TASK_LESION]["high_sensitivity_threshold"]
        lesion_hsp = ctx.runner.task_thresholds[TASK_LESION]["high_specificity_threshold"]
        pneu_hs = ctx.runner.task_thresholds[TASK_PNEUMONIA]["high_sensitivity_threshold"]
        pneu_hsp = ctx.runner.task_thresholds[TASK_PNEUMONIA]["high_specificity_threshold"]

        task_decisions = {
            TASK_LESION: {
                "highSensitivity": cancer_probability >= lesion_hs,
                "highSpecificity": cancer_probability >= lesion_hsp,
            },
            TASK_PNEUMONIA: {
                "highSensitivity": pneumonia_probability >= pneu_hs,
                "highSpecificity": pneumonia_probability >= pneu_hsp,
            },
        }
        effective_stage = ctx.runner.clinical_stage if ctx.runner.governance_gate_passed else "RESEARCH_ONLY"

        return PredictResponse(
            modelVersion=ctx.runner.model_version,
            cancerProbability=float(np.clip(cancer_probability, 0.0, 1.0)),
            regions=regions,
            labelScores=merged_scores,
            infectionCoverage=infection_coverage,
            whiteLungAssessment=white_lung_assessment,
            topFindings=top_findings,
            calibrationTemperature=ctx.runner.calibration_temperature,
            decisionHighSensitivity=task_decisions[TASK_LESION]["highSensitivity"],
            decisionHighSpecificity=task_decisions[TASK_LESION]["highSpecificity"],
            taskDecisions=task_decisions,
            operatingPointsUsed={
                TASK_LESION: {
                    "highSensitivityThreshold": lesion_hs,
                    "highSpecificityThreshold": lesion_hsp,
                },
                TASK_PNEUMONIA: {
                    "highSensitivityThreshold": pneu_hs,
                    "highSpecificityThreshold": pneu_hsp,
                },
            },
            clinicalUse=effective_stage,
            clinicalStage=effective_stage,
            detectorModelLoaded=ctx.runner.detector_session is not None,
            heatmapPath=None,
        )

    return app
