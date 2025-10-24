FROM python:3.9-alpine

# Install dependencies for Ansible, SSH, and OpenSSL (for password hashing)
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

# Install API Python deps
COPY api/requirements.txt /app/api-requirements.txt
RUN pip install --no-cache-dir -r /app/api-requirements.txt

# Copy source (only what we need)
COPY common /app/src/common
COPY api /app/src/api

EXPOSE 8080
CMD ["python", "-m", "api.api"]
