FROM node:22-bookworm

RUN apt-get update && apt-get install -y \
    ffmpeg \
    chromium \
    fonts-noto-cjk \
    fonts-noto-color-emoji \
    && rm -rf /var/lib/apt/lists/*

ENV PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium
ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci

COPY tsconfig.json ./
COPY src ./src

RUN npm run build

ENV PORT=8787
ENV HOST=0.0.0.0
ENV HYPERFRAMES_PROJECT_ROOT=/data/aios/projects
ENV HYPERFRAMES_OUTPUT_ROOT=/data/aios/projects

EXPOSE 8787

CMD ["npm", "run", "start"]
