# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server
RUN CGO_ENABLED=0 go build -o healthcheck ./cmd/healthcheck

FROM scratch

COPY --from=builder /build/server /server
COPY --from=builder /build/healthcheck /healthcheck

EXPOSE 8090

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/healthcheck"]

ENTRYPOINT ["/server"]
