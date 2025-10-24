FROM python:3.9-alpine

RUN apk add --no-cache \
    build-base \
    openssh-client \
    py3-pip \
    ansible \
    sshpass \
    openssl

WORKDIR /app

ENV PYTHONUNBUFFERED=1
ENV PYTHONPATH=/app/src
ENV ANSIBLE_HOST_KEY_CHECKING=False

# Install API Python deps (now includes gunicorn)
COPY api/requirements.txt /app/api-requirements.txt
RUN pip install --no-cache-dir -r /app/api-requirements.txt

# Copy source
COPY common /app/src/common
COPY api /app/src/api

# Expose Gunicorn port
EXPOSE 8080

# Start Gunicorn with the Flask app factory
# Note: pointing at the factory: api.api:create_app()
CMD ["gunicorn", "-c", "/app/src/api/gunicorn.conf.py", "api.api:create_app()"]
