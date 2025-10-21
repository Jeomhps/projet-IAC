FROM python:3.9-alpine

# Install dependencies for Ansible and SSH
RUN apk add --no-cache \
    build-base \
    openssh-client \
    py3-pip \
    ansible \
    sshpass

RUN pip install --no-cache-dir passlib

ENV ANSIBLE_HOST_KEY_CHECKING=False

# Set workdir
WORKDIR /app

# Copy requirements and src folder
COPY requirements.txt /app/
COPY api.py /app/src/
COPY create-users.yml /app/src/

# Install Python requirements
RUN pip install --no-cache-dir -r /app/requirements.txt

# Expose API port
EXPOSE 8080

# Run your Flask API
CMD ["python", "src/api.py"]
