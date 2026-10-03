FROM ghcr.io/astral-sh/uv:0.12.22 AS uv

FROM python:3.13.16-slim AS build
COPY --from=uv /uv /usr/local/bin/uv
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy UV_PROJECT_ENVIRONMENT=/app/.venv
WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev --no-install-project
COPY src ./src
RUN uv sync --frozen --no-dev

FROM python:3.13.16-slim
RUN useradd --uid 10001 --create-home rag
COPY --from=build --chown=rag:rag /app /app
ENV PATH="/app/.venv/bin:$PATH"
USER rag
EXPOSE 8090
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:8090/healthz').status==200 else 1)"
ENTRYPOINT ["ragx"]
CMD ["serve"]
