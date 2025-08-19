# --- Build Stage ---
FROM golang:1.25-bookworm AS builder
# build the application
WORKDIR /usr/src/app/
# let's create separate layers for dependencies and source code
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY main.go main.go
RUN CGO_ENABLED=0 GOOS=linux GO111MODULE=on go build -ldflags "-s -w" -o dice-go main.go

# --- Run Stage ---
FROM gcr.io/distroless/static-debian12:nonroot

ARG REST_PORT=8081
ENV REST_PORT=${REST_PORT}

LABEL org.opencontainers.image.source=https://github.com/apeirora/audit-log-poc-for-otel

WORKDIR /usr/src/app/
COPY --from=builder /usr/src/app/dice-go/ ./

EXPOSE ${REST_PORT}
ENTRYPOINT [ "./dice-go" ]
