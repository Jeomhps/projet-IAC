FROM python:3.9-alpine

RUN apk add --no-cache \
    build-base \
    openssh-client \
    py3-pip \
    ansible \
    sshpass \
    openssl \
    tini

WORKDIR /app

ENV PYTHONUNBUFFERED=1
ENV PYTHONFAULTHANDLER=1
ENV PYTHONPATH=/app/src
ENV ANSIBLE_HOST_KEY_CHECKING=False

COPY api/requirements.txt /app/api-requirements.txt
RUN pip install --no-cache-dir -r /app/api-requirements.txt

COPY common /app/src/common
COPY api /app/src/api

EXPOSE 8080

ENTRYPOINT ["tini", "--"]
# Wait for DB once, then start Gunicorn
CMD ["sh", "-c", "python -m common.wait_for_db && exec gunicorn -c /app/src/api/gunicorn.conf.py 'api.api:create_app()'"]
