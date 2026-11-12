# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

FROM golang:1.27.4-alpine3.23@sha256:146e3b9c7c1e2ec3d3f5348c8a686151a75e1976e50efac9fc022ebe446a9a82 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/dined ./cmd/dined \
    && CGO_ENABLED=0 GOOS=linux go build -o /out/photo-storage ./cmd/photo-storage

FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
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
