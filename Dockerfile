FROM alpine:latest

# Install Python, SSH, and sudo
RUN apk add --no-cache python3 py3-pip openssh-server sudo bash && \
    echo 'root:yourpassword' | chpasswd && \
    sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config && \
    sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config

# Create an entrypoint script to start SSH and keep the container running
RUN echo '#!/bin/sh' > /entrypoint.sh && \
    echo '/usr/sbin/sshd -D &' >> /entrypoint.sh && \
    echo 'tail -f /dev/null' >> /entrypoint.sh && \
    chmod +x /entrypoint.sh

EXPOSE 22

# Use the entrypoint script
ENTRYPOINT ["/entrypoint.sh"]
