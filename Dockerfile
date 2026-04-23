FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/notification-api ./cmd/api

FROM gcr.io/distroless/base-debian12

WORKDIR /app
COPY --from=builder /out/notification-api /app/notification-api

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/app/notification-api"]
