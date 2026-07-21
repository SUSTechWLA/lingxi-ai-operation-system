FROM postgres:16-alpine

ENTRYPOINT []
WORKDIR /app

COPY build/tangying-ai-os ./tangying-ai-os
COPY skills ./skills
COPY skill-capabilities ./skill-capabilities
COPY video-pipelines ./video-pipelines

EXPOSE 8080

CMD ["./tangying-ai-os"]
