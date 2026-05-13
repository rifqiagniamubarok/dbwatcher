FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /dbwatch ./cmd/dbwatch

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /dbwatch /usr/local/bin/dbwatch
ENTRYPOINT ["dbwatch"]
