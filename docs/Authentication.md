# Authentication

Summary of how authentication works in OpenTelemetry Collector using server- and client-side authenticators.

## Modes

- Server side: collector intercepts inbound HTTP/gRPC, extracts headers or query params, calls the server authenticator, returns
  401/Unauthenticated on failure, forwards with enriched context on success.
- Client side: collector wraps outbound HTTP or gRPC with authenticator-provided credentials so every request carries required headers,
  tokens, or signing.

## Authenticator types

- Server (receivers): `basicauthextension`, `bearertokenauthextension`, `oidcauthextension`
- Client (exporters): `asapauthextension`, `basicauthextension`, `bearertokenauthextension`, `oauth2clientauthextension`,
  `sigv4authextension`

## Notes For example for client-side OAuth2 in the collector you need an OAuth2 authorization server that exposes a token endpoint. The
oauth2client extension is configured with that server’s token_url, your client_id/client_secret (and optional scopes/audience/TLS). It will
call the token endpoint to obtain/refresh access tokens and inject Authorization: Bearer <token> on exporter requests. Without a reachable
OAuth2 server, the extension cannot get tokens.

## Server-side example (basic auth)

```yaml
extensions:
  basicauth/server:
    htpasswd: ./htpasswd # file with user:hashed-password entries

receivers:
  otlp:
    protocols:
      http:
        auth:
          authenticator: basicauth/server
      grpc:
        auth:
          authenticator: basicauth/server

exporters:
  debug: {}

service:
  extensions: [basicauth/server]
  pipelines:
    traces:
      receivers: [otlp]
      processors: []
      exporters: [debug]
    metrics:
      receivers: [otlp]
      processors: []
      exporters: [debug]
```

## Client-side example (OAuth2)

```yaml
extensions:
  oauth2client:
    client_id: my-client
    client_secret: supersecret
    token_url: https://auth.example.com/oauth/token
    scopes: [metrics.write]

exporters:
  otlphttp:
    endpoint: https://backend.example.com
    auth:
      authenticator: oauth2client

service:
  extensions: [oauth2client]
  pipelines:
    metrics:
      receivers: [otlp]
      processors: []
      exporters: [otlphttp]
```

## References

- `opentelemetry-collector/config/configauth/README.md`
