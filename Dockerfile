FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY app/go.mod app/go.sum* ./

RUN go mod download
COPY /app ./

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mlflow-autostop .


FROM alpine:latest AS mlflow-autostop
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app/

COPY --from=builder /app/mlflow-autostop .
RUN adduser -D -H -h /app appuser
USER appuser


ENTRYPOINT ["/app/mlflow-autostop"]
