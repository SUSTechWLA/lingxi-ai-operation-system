FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY skills ./skills
COPY skill-capabilities ./skill-capabilities
RUN CGO_ENABLED=0 GOOS=linux go build -o /tangying-ai-os ./cmd/tangying-ai-os

FROM alpine:3.19

RUN apk --no-cache add ca-certificates bash curl
WORKDIR /app

COPY --from=builder /tangying-ai-os .
COPY --from=builder /app/skills ./skills
COPY --from=builder /app/skill-capabilities ./skill-capabilities

EXPOSE 8080

CMD ["./tangying-ai-os"]
