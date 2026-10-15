FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    curl \
    git \
    tmux \
    ca-certificates \
    openssh-client \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -m -s /bin/bash devin \
    && mkdir /workspace && chown devin:devin /workspace

USER devin
RUN curl -fsSL https://cli.devin.ai/install.sh | bash

ENV PATH="/home/devin/.local/bin:${PATH}"

WORKDIR /workspace
CMD ["sleep", "infinity"]
