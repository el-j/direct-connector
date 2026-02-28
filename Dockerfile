# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

# Install git (needed by some Go modules) and CA certs.
RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Pure-Go static binary — no CGO, no display libraries needed.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -tags nofyne -ldflags="-s -w" -o /direct-connector .

# ── Stage 2: minimal runtime image ────────────────────────────────────────────
FROM scratch

# CA certificates for STUN (TLS to stun.l.google.com) and SSH.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /direct-connector /direct-connector

# The app config dir (keys, config JSON, SDP exchange files).
VOLUME /config
ENV XDG_CONFIG_HOME=/config

ENTRYPOINT ["/direct-connector"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/direct-connector", "--help"]
