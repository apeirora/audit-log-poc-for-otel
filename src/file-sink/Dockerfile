# --- Build Stage ---
FROM golang:1.25-bookworm AS builder
# build the application
WORKDIR /usr/src/app/
RUN mkdir -p data
# let's create separate layers for dependencies and source code
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY main.go main.go
RUN CGO_ENABLED=0 GOOS=linux GO111MODULE=on go build -ldflags "-s -w" -o file-sink main.go

# --- Run Stage ---
FROM gcr.io/distroless/static-debian12:nonroot

ARG GRPC_PORT=5317
ENV GRPC_PORT=${GRPC_PORT}

ARG HTTP_PORT=5318
ENV HTTP_PORT=${HTTP_PORT}

ARG LOG_FILE=/data/received-logs.txt
ENV LOG_FILE=${LOG_FILE}

LABEL org.opencontainers.image.source=https://github.com/apeirora/audit-log-poc-for-otel

WORKDIR /usr/src/app/
COPY --from=builder /usr/src/app/file-sink/ ./
COPY --from=builder /usr/src/app/data/ /data/

VOLUME ["/data"]

EXPOSE ${GRPC_PORT} ${HTTP_PORT}
ENTRYPOINT [ "./file-sink" ]
