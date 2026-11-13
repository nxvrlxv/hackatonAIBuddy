FROM python:3.14.4-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app
COPY pyproject.toml README.md ./
COPY src ./src
RUN pip install --no-cache-dir . \
    && useradd --uid 10001 --create-home agent

USER agent
EXPOSE 8001
# Bind/workers/threads/timeout задаются через GUNICORN_CMD_ARGS.
CMD ["gunicorn", "main:create_app()"]
