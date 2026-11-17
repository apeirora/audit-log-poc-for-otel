FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /out/spammer ./cmd/spammer \
    && CGO_ENABLED=0 go build -trimpath -o /out/toxic ./cmd/toxic \
    && CGO_ENABLED=0 go build -trimpath -o /out/sink ./cmd/sink \
    && CGO_ENABLED=0 go build -trimpath -o /out/edge ./cmd/edge \
    && CGO_ENABLED=0 go build -trimpath -o /out/ui ./cmd/ui

FROM debian:bookworm-slim AS spammer
COPY --from=build /out/spammer /spammer
EXPOSE 8080
ENTRYPOINT ["/spammer"]

FROM debian:bookworm-slim AS toxic
COPY --from=build /out/toxic /toxic
EXPOSE 8090
ENTRYPOINT ["/toxic"]

FROM debian:bookworm-slim AS sink
COPY --from=build /out/sink /sink
EXPOSE 8080
ENTRYPOINT ["/sink"]

FROM debian:bookworm-slim AS ui
COPY --from=build /out/ui /ui
EXPOSE 8080
ENTRYPOINT ["/ui"]

FROM debian:bookworm-slim AS collector
ARG TARGETARCH=amd64
ARG OTELCOL_VERSION=0.128.0
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tar \
    && curl -fsSL -o /tmp/otel.tgz "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v${OTELCOL_VERSION}/otelcol-contrib_${OTELCOL_VERSION}_linux_${TARGETARCH}.tar.gz" \
    && tar -xzf /tmp/otel.tgz -C /usr/local/bin otelcol-contrib \
    && rm -rf /var/lib/apt/lists/* /tmp/otel.tgz
COPY --from=build /out/edge /usr/local/bin/edge
COPY collector/sidecar.yaml /etc/otel/sidecar.yaml
COPY collector/collector.yaml /etc/otel/collector.yaml
COPY collector/entrypoint.sh /entrypoint.sh
RUN sed -i 's/\r$//' /entrypoint.sh && chmod +x /entrypoint.sh /usr/local/bin/edge /usr/local/bin/otelcol-contrib
ENTRYPOINT ["/entrypoint.sh"]
