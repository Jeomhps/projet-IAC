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
ENV PYTHONPATH=/app/src
ENV ANSIBLE_HOST_KEY_CHECKING=False

COPY scheduler/requirements.txt /app/scheduler-requirements.txt
RUN pip install --no-cache-dir -r /app/scheduler-requirements.txt

COPY common /app/src/common
COPY scheduler /app/src/scheduler

ENTRYPOINT ["tini", "--"]
# Wait for DB once, then run the scheduler
CMD ["sh", "-c", "python -m common.wait_for_db && exec python -m scheduler.maintenance --interval 60"]
