FROM kong:3.9.0-ubuntu

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/unchartedsky/sonic-boom"

USER root

RUN ln -snf /usr/share/zoneinfo/Asia/Seoul /etc/localtime \
    && echo "Asia/Seoul" > /etc/timezone

RUN DEBIAN_FRONTEND=noninteractive apt-get update -y \
    && apt-get upgrade -y \
    && apt-get install -y \
        curl wget jq vim inotify-tools \
    && rm -rf /var/lib/apt/lists/*

# See https://subvars.lmno.pk/01-installation/
ENV SUBVARS_VERSION "0.1.5"
RUN wget -q https://github.com/kha7iq/subvars/releases/download/v${SUBVARS_VERSION}/subvars_${TARGETARCH}.deb \
    && dpkg --install subvars_${TARGETARCH}.deb \
    && rm -f subvars_${TARGETARCH}.deb

COPY bin/${TARGETOS}-${TARGETARCH}/sonic-boom /kong/go-plugins/sonic-boom

RUN mkdir -p /var/log/kong \
    && chown -R kong:kong /var/log/kong \
    && chown -R kong:kong /kong \
    && chmod +x /kong/go-plugins/sonic-boom

USER kong

ENV KONG_PLUGINSERVER_NAMES "sonic-boom"

ENV KONG_PLUGINS "bundled,sonic-boom"
ENV KONG_PLUGINSERVER_NAMES "sonic-boom"
ENV KONG_PLUGINSERVER_SONIC_BOOM_START_CMD "/kong/go-plugins/sonic-boom"
ENV KONG_PLUGINSERVER_SONIC_BOOM_QUERY_CMD "/kong/go-plugins/sonic-boom -dump"
ENV KONG_DATABASE "off"
