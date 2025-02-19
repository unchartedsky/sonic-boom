FROM kong:3.9.0-ubuntu

ARG TARGETOS
ARG TARGETARCH

LABEL org.opencontainers.image.source="https://github.com/unchartedsky/sonic-boom"

USER root

RUN ln -snf /usr/share/zoneinfo/Asia/Seoul /etc/localtime \
    && echo "Asia/Seoul" > /etc/timezone

RUN DEBIAN_FRONTEND=noninteractive apt-get update -y \
    && apt-get upgrade -y \
    && apt-get install -y --no-install-recommends \
    curl wget jq vim inotify-tools build-essential \
    && luarocks install lua-cjson \
    && apt-get remove -y build-essential \
    && apt-get autoremove -y \
    && apt-get clean \
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

# Pass OpenTelemetry environment variables to nginx
# See https://discuss.konghq.com/t/set-multiple-env-nginx-directives/7532/3
ENV KONG_NGINX_MAIN_ENV "OTEL_SDK_DISABLED;env OTEL_SERVICE_NAME;env OTEL_EXPORTER_OTLP_ENDPOINT;env OTEL_EXPORTER_OTLP_PROTOCOL;env OTEL_TRACES_SAMPLER;env OTEL_TRACES_SAMPLER_ARG;env OTEL_PROPAGATORS;env OTEL_RESOURCE_ATTRIBUTES;env OTEL_LOG_LEVEL"

# OpenTelemetry Configuration
## OpenTelemetry 활성화 여부 (true/false)
ENV OTEL_SDK_DISABLED="true"

## 서비스 이름 설정
ENV OTEL_SERVICE_NAME="sonic-boom"
## OpenTelemetry Collector의 엔드포인트
ENV OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4317"
ENV OTEL_EXPORTER_OTLP_PROTOCOL="grpc"
## 샘플링 비율, 1.0은 100% 샘플링을 의미
ENV OTEL_TRACES_SAMPLER="parentbased_always_on"
ENV OTEL_TRACES_SAMPLER_ARG="1.0"
ENV OTEL_PROPAGATORS="tracecontext,baggage"
ENV OTEL_RESOURCE_ATTRIBUTES="service.name=sonic-boom,service.version=${VERSION:-dev}"
## 선택 가능 "debug", "info", "warn", "error"
ENV OTEL_LOG_LEVEL="info"

# Kong Configuration
ENV KONG_PLUGINS "bundled,sonic-boom"
ENV KONG_PLUGINSERVER_NAMES "sonic-boom"
ENV KONG_PLUGINSERVER_SONIC_BOOM_START_CMD "/kong/go-plugins/sonic-boom"
ENV KONG_PLUGINSERVER_SONIC_BOOM_QUERY_CMD "/kong/go-plugins/sonic-boom -dump"
ENV KONG_DATABASE "off"
