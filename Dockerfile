# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG GIT_SHA=dev
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-X main.Version=${GIT_SHA} -X main.BuildTime=${BUILD_TIME}" \
      -o /app/space-invaders-backend .

# Run stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/space-invaders-backend .

# Platform sets PORT at runtime (Fly, Railway, Koyeb)
EXPOSE 8080
ENV PORT=8080
CMD ["./space-invaders-backend"]
