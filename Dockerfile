FROM golang:1.24-alpine
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /review-action .
ENTRYPOINT ["/review-action"]
