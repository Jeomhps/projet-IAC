# ======================
# 1) Builder: build wheels for Python deps
# ======================
FROM python:3.9-alpine AS builder

WORKDIR /app

RUN apk add --no-cache \
    build-base \
    openssl

COPY api/requirements.txt /app/api-requirements.txt

RUN pip wheel --wheel-dir /wheels -r /app/api-requirements.txt


# ======================
# 2) Runtime: only what we need to run the API
# ======================
FROM python:3.9-alpine

WORKDIR /app

RUN apk add --no-cache \
    openssh-client \
    ansible \
    sshpass \
    openssl \
    tini

ENV PYTHONUNBUFFERED=1 \
    PYTHONFAULTHANDLER=1 \
    PYTHONPATH=/app/src \
    ANSIBLE_HOST_KEY_CHECKING=False

COPY --from=builder /wheels /wheels
COPY api/requirements.txt /app/api-requirements.txt
RUN pip install --no-cache-dir --no-index --find-links=/wheels -r /app/api-requirements.txt \
    && rm -rf /wheels

COPY common /app/src/common
COPY api /app/src/api

EXPOSE 8080

ENTRYPOINT ["tini", "--"]
CMD ["sh", "-c", "python -m common.wait_for_db && exec gunicorn -c /app/src/api/gunicorn.conf.py 'api.api:create_app()'"]
