# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

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
        -o /out/sela-migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/sela-api /usr/local/bin/sela-api
COPY --from=builder /out/sela-migrate /usr/local/bin/sela-migrate

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/sela-api"]
