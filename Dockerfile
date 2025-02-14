FROM kong:3.9.0-ubuntu

LABEL org.opencontainers.image.source="https://github.com/unchartedsky/sonic-boom"

USER root

RUN apt-get update && apt-get install -y \
  curl wget jq \
  && rm -rf /var/lib/apt/lists/*

# See https://subvars.lmno.pk/01-installation/
ENV SUBVARS_VERSION="0.1.5"
RUN wget -q https://github.com/kha7iq/subvars/releases/download/v${SUBVARS_VERSION}/subvars_Linux_x86_64.tar.gz && \
  tar -xf subvars_Linux_x86_64.tar.gz && \
  chmod +x subvars && \
  mv subvars /usr/local/bin/subvars

# See https://github.com/hbagdi/hupit
ENV HUPIT_VERSION="0.1.0"
# https://github.com/hbagdi/hupit/releases/download/v0.1.0/hupit_0.1.0_linux_amd64.tar.gz
RUN wget -q https://github.com/hbagdi/hupit/releases/download/v${HUPIT_VERSION}/hupit_${HUPIT_VERSION}_linux_amd64.tar.gz && \
  tar -xf hupit_${HUPIT_VERSION}_linux_amd64.tar.gz && \
  chmod +x hupit && \
  mv hupit /usr/local/bin/hupit

COPY bin/linux-amd64/sonic-boom /kong/go-plugins/sonic-boom

RUN mkdir -p /var/log/kong \
    && chown -R kong:kong /var/log/kong \
    && chown -R kong:kong /kong

USER kong

ENV KONG_PLUGINSERVER_NAMES "sonic-boom"

ENV KONG_PLUGINS "bundled,sonic-boom"
ENV KONG_PLUGINSERVER_NAMES "sonic-boom"
ENV KONG_PLUGINSERVER_SONIC_BOOM_START_CMD "/kong/go-plugins/sonic-boom"
ENV KONG_PLUGINSERVER_SONIC_BOOM_QUERY_CMD "/kong/go-plugins/sonic-boom -dump"
ENV KONG_DATABASE "off"
