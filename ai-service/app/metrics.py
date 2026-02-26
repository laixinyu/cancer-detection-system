import time
from dataclasses import dataclass

from fastapi import FastAPI, Request
from fastapi.responses import PlainTextResponse
from prometheus_client import CollectorRegistry, Counter, Gauge, Histogram, generate_latest


@dataclass
class Metrics:
    registry: CollectorRegistry
    http_requests_total: Counter
    http_request_latency_seconds: Histogram
    predict_inflight: Gauge
    predict_infer_seconds: Histogram



def build_metrics() -> Metrics:
    registry = CollectorRegistry()
    return Metrics(
        registry=registry,
        http_requests_total=Counter(
            "ai_service_http_requests_total",
            "Total HTTP requests",
            ["method", "path", "status"],
            registry=registry,
        ),
        http_request_latency_seconds=Histogram(
            "ai_service_http_request_latency_seconds",
            "HTTP request latency in seconds",
            ["method", "path"],
            buckets=(0.01, 0.03, 0.05, 0.1, 0.3, 0.5, 1, 2, 5, 10, 30, 60),
            registry=registry,
        ),
        predict_inflight=Gauge(
            "ai_service_predict_inflight",
            "In-flight /predict requests",
            registry=registry,
        ),
        predict_infer_seconds=Histogram(
            "ai_service_predict_inference_seconds",
            "Inference execution time in seconds",
            buckets=(0.01, 0.03, 0.05, 0.1, 0.3, 0.5, 1, 2, 5, 10, 30, 60),
            registry=registry,
        ),
    )



def register_metrics_routes(app: FastAPI) -> None:
    metrics_state: Metrics = app.state.ctx.metrics

    @app.get("/metrics")
    def metrics() -> PlainTextResponse:
        return PlainTextResponse(generate_latest(metrics_state.registry).decode("utf-8"))



def register_metrics_middleware(app: FastAPI, metrics: Metrics) -> None:
    @app.middleware("http")
    async def request_metrics_middleware(request: Request, call_next):
        start = time.perf_counter()
        status_code = 500
        try:
            response = await call_next(request)
            status_code = response.status_code
            return response
        finally:
            duration = time.perf_counter() - start
            method = request.method
            path = request.url.path
            metrics.http_request_latency_seconds.labels(method=method, path=path).observe(duration)
            metrics.http_requests_total.labels(method=method, path=path, status=str(status_code)).inc()
