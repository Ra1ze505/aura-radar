FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /aura-radar ./src

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates && mkdir -p /app/data
COPY --from=builder /aura-radar /app/aura-radar
CMD ["/app/aura-radar"]
