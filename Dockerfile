# syntax=docker/dockerfile:1

FROM golang:1.26.11-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev

RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/sela-api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/sela-migrate ./cmd/migrate \
    && CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/sela-worker ./cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/sela-api /usr/local/bin/sela-api
COPY --from=builder /out/sela-migrate /usr/local/bin/sela-migrate
COPY --from=builder /out/sela-worker /usr/local/bin/sela-worker

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/sela-api"]
