FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY app/go.mod app/go.sum* ./

# Download dependencies (if you have any external dependencies)
RUN go mod download

# Copy source code
COPY /app ./

# Build the application with static linking
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mlflow-autostop .

# Create a minimal image
FROM alpine:latest AS mlflow-autostop

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app/

# Copy the binary from the builder stage
COPY --from=builder /app/mlflow-autostop .

# Create a non-root user to run the application
RUN adduser -D -H -h /app appuser
USER appuser

# Command to run
ENTRYPOINT ["/app/mlflow-autostop"]
