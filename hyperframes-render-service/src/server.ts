/**
 * HyperFrames Render Service — HTTP server entry point.
 *
 * Exposes:
 *   GET  /health          — readiness probe
 *   POST /render          — sync MP4 render
 *   POST /lint            — lint project HTML (Phase 1: basic checks)
 *   POST /snapshot        — keyframe snapshots (Phase 2)
 *   POST /render/stream   — SSE progress stream (Phase 2)
 *   GET  /jobs/:jobId     — job status
 *
 * AIOS Go backend calls these instead of shelling out to npx hyperframes.
 */

import Fastify from "fastify";
import { z } from "zod";
import { renderVideo } from "./render.js";
import { lintProject } from "./lint.js";
import { takeSnapshots } from "./snapshot.js";
import { getJobProgress, listJobIds } from "./queue.js";
import type { SecurityConfig } from "./security.js";
import type { RenderRequest, RenderProgress } from "./types.js";

// ── Security config ──────────────────────────────────────────────────

const security: SecurityConfig = {
  allowedProjectRoots: [
    process.env.HYPERFRAMES_PROJECT_ROOT ?? "/data/aios/projects",
  ],
  allowedOutputRoots: [
    process.env.HYPERFRAMES_OUTPUT_ROOT ?? "/data/aios/projects",
  ],
};

// ── Zod schemas ──────────────────────────────────────────────────────

const renderSchema = z.object({
  projectDir: z.string().min(1),
  entry: z.string().optional(),
  outputPath: z.string().min(1),
  fps: z.number().int().min(1).max(120).optional(),
  quality: z.enum(["draft", "standard", "high"]).optional(),
  format: z.enum(["mp4", "webm", "mov", "png-sequence"]).optional(),
  workers: z.number().int().min(1).max(8).optional(),
  useGpu: z.boolean().optional(),
});

const lintSchema = z.object({
  projectDir: z.string().min(1),
  entry: z.string().optional(),
});

const snapshotSchema = z.object({
  projectDir: z.string().min(1),
  entry: z.string().optional(),
  times: z.array(z.number().nonnegative()),
  outputDir: z.string().min(1),
});

// ── App ──────────────────────────────────────────────────────────────

const app = Fastify({ logger: true });

// ── Routes ───────────────────────────────────────────────────────────

// GET /health — readiness probe for AIOS startup check and diagnostics.
app.get("/health", async () => {
  // Best-effort dependency detection.
  let ffmpeg = false;
  let chromium = false;
  let producer = true; // assume true if the process can import it

  try {
    const { execSync } = await import("node:child_process");
    execSync("ffmpeg -version", { stdio: "ignore", timeout: 3000 });
    ffmpeg = true;
  } catch {
    // ffmpeg not on PATH — not fatal for all render modes
  }

  try {
    const { execSync } = await import("node:child_process");
    execSync("chromium --version", { stdio: "ignore", timeout: 3000 });
    chromium = true;
  } catch {
    try {
      const { execSync } = await import("node:child_process");
      execSync("google-chrome --version", { stdio: "ignore", timeout: 3000 });
      chromium = true;
    } catch {
      // chrome/chromium not on PATH
    }
  }

  return {
    ok: true,
    service: "hyperframes-render-service",
    version: "1.0.0",
    dependencies: {
      node: process.version,
      ffmpeg,
      chromium,
      producer,
    },
  };
});

// POST /render — synchronous MP4 render via @hyperframes/producer.
app.post("/render", async (request, reply) => {
  const parsed = renderSchema.safeParse(request.body);
  if (!parsed.success) {
    return reply.code(400).send({
      ok: false,
      error: parsed.error.flatten(),
    });
  }

  const result = await renderVideo(parsed.data as RenderRequest, security);
  return reply.code(result.ok ? 200 : 500).send(result);
});

// POST /lint — validate a HyperFrames project for common issues.
app.post("/lint", async (request, reply) => {
  const parsed = lintSchema.safeParse(request.body);
  if (!parsed.success) {
    return reply.code(400).send({
      ok: false,
      error: parsed.error.flatten(),
    });
  }

  const result = await lintProject(parsed.data, security);
  return reply.code(result.ok ? 200 : 200).send(result); // always 200 — errors are lint findings
});

// POST /snapshot — take keyframe screenshots at specific timestamps.
app.post("/snapshot", async (request, reply) => {
  const parsed = snapshotSchema.safeParse(request.body);
  if (!parsed.success) {
    return reply.code(400).send({
      ok: false,
      error: parsed.error.flatten(),
    });
  }

  const result = await takeSnapshots(parsed.data, security);
  return reply.code(result.ok ? 200 : 500).send(result);
});

// GET /jobs/:jobId — poll job progress.
app.get<{ Params: { jobId: string } }>("/jobs/:jobId", async (request, reply) => {
  const progress = getJobProgress(request.params.jobId);
  if (!progress) {
    return reply.code(404).send({ ok: false, error: "Job not found" });
  }
  return progress;
});

// GET /jobs — list recent job IDs.
app.get("/jobs", async () => {
  return { jobs: listJobIds() };
});

// POST /render/stream — SSE endpoint for streaming render progress.
// Phase 2: full SSE streaming. Phase 1: returns the current job state.
app.post("/render/stream", async (request, reply) => {
  const parsed = renderSchema.safeParse(request.body);
  if (!parsed.success) {
    return reply.code(400).send({
      ok: false,
      error: parsed.error.flatten(),
    });
  }

  // Start the render asynchronously and stream progress via SSE.
  reply.raw.writeHead(200, {
    "Content-Type": "text/event-stream",
    "Cache-Control": "no-cache",
    Connection: "keep-alive",
  });

  const sendSSE = (event: string, data: unknown) => {
    reply.raw.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
  };

  try {
    const result = await renderVideo(parsed.data as RenderRequest, security);
    sendSSE("complete", result);
  } catch (err) {
    sendSSE("error", {
      ok: false,
      error: err instanceof Error ? err.message : String(err),
    });
  }

  reply.raw.end();
});

// ── Start ────────────────────────────────────────────────────────────

const port = Number(process.env.PORT ?? 8787);
const host = process.env.HOST ?? "127.0.0.1";

app.listen({ port, host }, (err, address) => {
  if (err) {
    console.error("Failed to start HyperFrames Render Service:", err);
    process.exit(1);
  }
  console.log(`HyperFrames Render Service listening at ${address}`);
});

export { app };
