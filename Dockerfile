# 빌드 스테이지
FROM kong:3.9.0-ubuntu AS builder

USER root

# 필요한 도구 설치
RUN DEBIAN_FRONTEND=noninteractive apt-get update -y \
    && apt-get upgrade -y \
    && apt-get install -y \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# kong-plugin-template-transformer 저장소 클론
RUN git clone https://github.com/stone-payments/kong-plugin-template-transformer.git

# 플러그인 디렉토리로 이동
WORKDIR /kong-plugin-template-transformer

# 플러그인 빌드
RUN make && make install

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

# https://github.com/stone-payments/kong-plugin-template-transformer 설치
## 빌드 스테이지에서 플러그인 파일 복사
COPY --from=builder /kong-plugin-template-transformer/template-transformer /usr/local/share/lua/5.1/kong/plugins/template-transformer

## 플러그인을 Kong 설정에 추가
# RUN echo "custom_plugins = template-transformer" >> /etc/kong/kong.conf.default

COPY bin/${TARGETOS}-${TARGETARCH}/sonic-boom /kong/go-plugins/sonic-boom

RUN mkdir -p /var/log/kong \
    && chown -R kong:kong /var/log/kong \
    && chown -R kong:kong /kong \
    && chmod +x /kong/go-plugins/sonic-boom

USER kong

ENV KONG_PLUGINSERVER_NAMES "sonic-boom"

ENV KONG_PLUGINS "bundled,template-transformer,sonic-boom"
ENV KONG_PLUGINSERVER_NAMES "sonic-boom"
ENV KONG_PLUGINSERVER_SONIC_BOOM_START_CMD "/kong/go-plugins/sonic-boom"
ENV KONG_PLUGINSERVER_SONIC_BOOM_QUERY_CMD "/kong/go-plugins/sonic-boom -dump"
ENV KONG_DATABASE "off"
