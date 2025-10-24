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

# Install scheduler Python deps (no Flask)
COPY scheduler/requirements.txt /app/scheduler-requirements.txt
RUN pip install --no-cache-dir -r /app/scheduler-requirements.txt

# Copy source (shared common + scheduler)
COPY common /app/src/common
COPY scheduler /app/src/scheduler

CMD ["python", "-m", "scheduler.maintenance", "--interval", "60"]
