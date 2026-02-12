FROM quay.io/fedora/fedora-bootc:41

LABEL containers.bootc=1

# Install packages
RUN dnf install -y vim htop && dnf clean all
