FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/llmpool ./cmd/api

FROM alpine:3.21

WORKDIR /app

COPY --from=builder /out/llmpool /usr/local/bin/llmpool
COPY configs/default.yml /app/configs/default.yml

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/llmpool"]
