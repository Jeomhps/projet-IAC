FROM python:3.9-alpine

# Install dependencies for Ansible, SSH, and OpenSSL (for password hashing)
RUN apk add --no-cache \
    build-base \
    openssh-client \
    py3-pip \
    ansible \
    sshpass \
    openssl

# Python dependencies
# Use PyMySQL for MySQL/MariaDB driver (pure Python, works well on Alpine)
# passlib is used in Ansible password hashing scenario if needed
RUN pip install --no-cache-dir passlib

ENV ANSIBLE_HOST_KEY_CHECKING=False

# Set workdir
WORKDIR /app

# Copy requirements and install
COPY requirements.txt /app/
RUN pip install --no-cache-dir -r /app/requirements.txt

# Copy application source (entire src)
COPY . /app/src/

# Expose API port
EXPOSE 8080

# Run the Flask API
CMD ["python", "src/api.py"]
