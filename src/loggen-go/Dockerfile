FROM golang:1.24-bookworm AS builder

WORKDIR /usr/src/app/

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY main.go main.go

RUN CGO_ENABLED=0 GOOS=linux GO111MODULE=on go build -ldflags "-s -w" -o loggen-go main.go

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /usr/src/app/

LABEL org.opencontainers.image.source=https://github.com/apeirora/audit-log-poc-for-otel

COPY --from=builder /usr/src/app/loggen-go/ ./

ENTRYPOINT [ "./loggen-go" ]
