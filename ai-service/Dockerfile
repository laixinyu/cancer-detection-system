FROM python:3.11-slim

WORKDIR /app

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV PIP_DISABLE_PIP_VERSION_CHECK=1
ENV PIP_DEFAULT_TIMEOUT=300
ENV PIP_RETRIES=10

ARG PIP_INDEX_URL=https://pypi.org/simple
ARG PIP_EXTRA_INDEX_URL=
ENV PIP_INDEX_URL=${PIP_INDEX_URL}
ENV PIP_EXTRA_INDEX_URL=${PIP_EXTRA_INDEX_URL}
ENV AI_REQUIRE_API_KEY=true
ENV AI_ENFORCE_GOVERNANCE_GATE=true
ENV AI_MAX_CONCURRENT_REQUESTS=2
ENV AI_INFERENCE_TIMEOUT_SECONDS=45
ENV AI_MAX_QUEUE_WAIT_SECONDS=2

COPY requirements.txt .
RUN python -m pip install --upgrade pip setuptools wheel \
  && pip install --prefer-binary --no-cache-dir -r requirements.txt

COPY app ./app
COPY models ./models

RUN useradd -m -u 10001 appuser \
  && chown -R appuser:appuser /app
USER appuser

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/livez', timeout=3)"

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "1"]
