FROM alpine:latest

# Install Python, SSH, sudo, and bash
RUN apk add --no-cache python3 py3-pip openssh-server sudo bash && \
    mkdir -p /run/sshd && \
    ssh-keygen -A && \  # Generate SSH host keys
    echo 'root:yourpassword' | chpasswd && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config && \
    sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config

# Create and set up the entrypoint script
RUN echo '#!/bin/sh' > /entrypoint.sh && \
    echo '/usr/sbin/sshd -D &' >> /entrypoint.sh && \
    echo 'tail -f /dev/null' >> /entrypoint.sh && \
    chmod +x /entrypoint.sh

EXPOSE 22
ENTRYPOINT ["/entrypoint.sh"]
