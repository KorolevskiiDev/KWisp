# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server

FROM scratch

COPY --from=builder /build/server /server

EXPOSE 8090

ENTRYPOINT ["/server"]
