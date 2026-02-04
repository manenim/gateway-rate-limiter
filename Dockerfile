# Build Stage
FROM golang:1.24.5-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/example-server

# Run Stage
FROM gcr.io/distroless/static-debian11
COPY --from=builder /app/server /
CMD ["/server"]
