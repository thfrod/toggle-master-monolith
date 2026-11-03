FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/evaluation-service .

FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
COPY --from=builder /out/evaluation-service /usr/local/bin/evaluation-service
USER app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s CMD wget -qO- http://127.0.0.1:8080/health >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/evaluation-service"]
