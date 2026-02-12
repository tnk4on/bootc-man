FROM quay.io/fedora/fedora-bootc:41

LABEL containers.bootc=1
LABEL maintainer="test@example.com"

ARG VERSION=1.0.0

# Create user
RUN useradd -m -G wheel testuser && \
    echo "testuser ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/testuser

# SSH setup
RUN mkdir -p /home/testuser/.ssh && \
    chmod 700 /home/testuser/.ssh && \
    chown testuser:testuser /home/testuser/.ssh

# Install packages
RUN dnf install -y \
    vim-enhanced \
    htop \
    tmux \
    git \
    && dnf clean all

# Copy configuration
COPY config/ /etc/myapp/

# Set version label
LABEL version=${VERSION}
