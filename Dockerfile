FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /review-action .

FROM alpine:3.19
RUN apk add --no-cache git ca-certificates
COPY --from=builder /review-action /review-action
ENTRYPOINT ["/review-action"]
