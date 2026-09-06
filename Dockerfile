# syntax=docker/dockerfile:1

FROM golang:1.26.6-alpine3.23 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/dined ./cmd/dined

FROM alpine:3.20
WORKDIR /app

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /out/dined ./dined
COPY --from=builder /src/static ./static
COPY migrations ./migrations

RUN addgroup -S dined \
    && adduser -S -G dined dined \
    && chown -R dined:dined /app

ENV PORT=4600
ENV LOG_LEVEL=info
USER dined

EXPOSE 4600
CMD ["./dined"]
