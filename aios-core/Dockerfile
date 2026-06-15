FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -o /tangying-ai-os cmd/tangying-ai-os/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates bash
WORKDIR /app

COPY --from=builder /tangying-ai-os .
COPY .env.example .env

EXPOSE 8080

CMD ["./tangying-ai-os"]
