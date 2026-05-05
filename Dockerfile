FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /lingxi-ai-os cmd/lingxi-ai-os/main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates bash
WORKDIR /app

COPY --from=builder /lingxi-ai-os .
COPY .env.example .env

EXPOSE 8080

CMD ["./lingxi-ai-os"]
