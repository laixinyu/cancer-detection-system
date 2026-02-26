# AI Service Production Blueprint

## Goals

1. Build a production-safe inference service with clear readiness/liveness behavior.
2. Make model rollout auditable with governance gates and staged promotion.
3. Improve latency stability and protect service under burst traffic.
4. Enable measurable SLO operations and incident response.

## Current Critical Risks (Reviewed)

1. Monolithic service logic in one file limits maintainability and safe iteration.
2. Runtime lacked full production guardrails (queue limits, timeout budgets, metrics surface).
3. Health semantics did not fully represent prediction readiness in all failure modes.
4. Deployment baseline lacked container hardening and explicit runtime defaults.

## Changes Implemented Now

1. Runtime safety:
`AI_MAX_CONCURRENT_REQUESTS`, inference timeout, queue wait timeout, bounded in-flight requests.
2. Observability:
`/metrics` with request count, latency, in-flight, and inference duration metrics.
3. Probe semantics:
`/livez` for liveness, `/health` for readiness + governance block reason.
4. Security/governance:
Optional API key gate, governance gate enforcement at predict path.
5. Container baseline:
non-root runtime user, explicit healthcheck, production env defaults in image.

## Deployment Architecture (Target)

1. Inference plane:
single-worker FastAPI process per pod (avoid duplicate model memory/GPU contention).
2. Routing plane:
gateway performs auth/JWT + idempotency + tracing, AI service remains inference-focused.
3. Model artifact plane:
versioned model registry bucket + immutable model digest (sha256) + signed metadata.
4. Control plane:
governance service controls stage (`RESEARCH_ONLY` -> `PILOT_DECISION_SUPPORT` -> `CLINICAL_DECISION_SUPPORT`).

## Model Deployment Strategy

1. Artifact contract:
`model.onnx`, `detector.onnx`, `manifest.json`, `governance.json`, `clinical_config.json`.
2. Promotion pipeline:
`train -> evaluate -> gate-check -> register -> canary -> full rollout`.
3. Release policy:
canary 5% for 30 minutes, then 25%, then 100% if SLO + quality KPIs pass.
4. Rollback:
instant revert to previous model digest when p95 latency, 5xx, or quality sentinel breaches.

## Inference Service Refactor Plan (Next)

1. Split modules:
`config.py`, `model_runtime.py`, `preprocess.py`, `postprocess.py`, `api.py`, `metrics.py`.
2. Define typed config:
replace direct env reads with one validated settings model.
3. Add test layers:
unit tests (pre/post), integration tests (/predict contract), load smoke tests.
4. Add offline/online parity:
same preprocessing package used by training export and online inference.

## SLO and Monitoring

1. Availability:
99.9% monthly for `/predict`.
2. Latency:
p95 < 1.5s, p99 < 3s (CPU baseline; GPU profile separately).
3. Correctness guardrails:
score distribution drift, class-wise alert thresholds, detector startup check status.
4. Alerting:
5xx rate, queue saturation, timeout rate, governance block spikes, model reload failure.

## Immediate Operational Checklist

1. Set production env:
`AI_REQUIRE_API_KEY=true`, `AI_API_KEY=<secret>`, `AI_ENFORCE_GOVERNANCE_GATE=true`.
2. Tune concurrency/timeouts by hardware:
start with `AI_MAX_CONCURRENT_REQUESTS=2`, adjust after load test.
3. Add dashboard:
HTTP latency/error + inference histogram + in-flight gauge + readiness status.
4. Add canary runbook:
who approves, KPI thresholds, rollback command, communication steps.
