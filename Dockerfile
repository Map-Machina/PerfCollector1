# Build stage
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build all binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/perfapi ./cmd/perfapi
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/perfprocessord ./cmd/perfprocessord
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/perfjournal ./cmd/perfjournal

# API server image
FROM alpine:3.19 AS perfapi

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/perfapi /usr/local/bin/perfapi

EXPOSE 8080

ENV PERFAPI_LISTEN=":8080"
ENV PERFAPI_DB_URI="user=postgres dbname=performancedata host=localhost sslmode=disable"

ENTRYPOINT ["perfapi"]

# Processor daemon image
FROM alpine:3.19 AS perfprocessord

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/perfprocessord /usr/local/bin/perfprocessord

EXPOSE 2222

ENTRYPOINT ["perfprocessord"]

# Journal tool image
FROM alpine:3.19 AS perfjournal

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/perfjournal /usr/local/bin/perfjournal

ENTRYPOINT ["perfjournal"]
