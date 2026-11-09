# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

FROM golang:1.27.3-alpine3.23@sha256:8a5046497cda39860b832ffdbe8dd66018a0f05bb01352f533a36e7e2ced66a0 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/dined ./cmd/dined \
    && CGO_ENABLED=0 GOOS=linux go build -o /out/photo-storage ./cmd/photo-storage

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
WORKDIR /app

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /out/dined ./dined
COPY --from=builder /out/photo-storage ./photo-storage
COPY --from=builder /src/static ./static
COPY migrations ./migrations

RUN addgroup -S dined \
    && adduser -S -G dined dined \
    && chown -R dined:dined /app

ENV PORT=4600
ENV LOG_LEVEL=info
USER dined

EXPOSE 4600
# Startup applies migrations before listening (up to ~75s), so allow a 90s grace period.
HEALTHCHECK --interval=30s --timeout=5s --start-period=90s --retries=3 \
    CMD wget -q -O - "http://127.0.0.1:${PORT:-4600}/health" || exit 1
CMD ["./dined"]
