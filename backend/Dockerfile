# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api \
 && CGO_ENABLED=0 go build -o /out/ingestor ./cmd/ingestor \
 && CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata tini
WORKDIR /app
COPY --from=build /out/ /usr/local/bin/
COPY migrations /app/migrations
ENV MIGRATIONS_DIR=/app/migrations
ENTRYPOINT ["/sbin/tini", "--"]
EXPOSE 8080