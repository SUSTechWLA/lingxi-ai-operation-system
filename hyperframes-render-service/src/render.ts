/**
 * Core rendering logic using @hyperframes/producer.
 *
 * Delegates frame rendering + FFmpeg encoding to the HyperFrames producer
 * library so we never shell out to `npx hyperframes render`.
 */

import { createRenderJob, executeRenderJob } from "@hyperframes/producer";
import { nanoid } from "nanoid";
import { assertPathAllowed, ensureDir, type SecurityConfig } from "./security.js";
import type { RenderRequest, RenderResult, RenderProgress } from "./types.js";
import { updateJobProgress } from "./queue.js";
import path from "node:path";

export async function renderVideo(
  req: RenderRequest,
  security: SecurityConfig,
): Promise<RenderResult> {
  const startedAt = Date.now();
  const jobId = `render_${nanoid(12)}`;

  try {
    const projectDir = assertPathAllowed(
      req.projectDir,
      security.allowedProjectRoots,
      "projectDir",
    );
    const outputPath = assertPathAllowed(
      req.outputPath,
      security.allowedOutputRoots,
      "outputPath",
    );

    // Ensure output directory exists.
    ensureDir(path.dirname(outputPath));

    // Report initial progress.
    updateJobProgress(jobId, { jobId, status: "preparing", progress: 0 });

    const workers = Math.min(Math.max(req.workers ?? 2, 1), 8);

    const job = createRenderJob({
      fps: req.fps ?? 30,
      quality: req.quality ?? "standard",
      format: req.format ?? "mp4",
      workers,
      useGpu: req.useGpu ?? false,
      debug: false,
    });

    updateJobProgress(jobId, {
      jobId,
      status: "rendering",
      progress: 30,
    });

    await executeRenderJob(job, projectDir, outputPath);

    updateJobProgress(jobId, {
      jobId,
      status: "encoding",
      progress: 88,
    });

    // Final progress update.
    updateJobProgress(jobId, {
      jobId,
      status: "complete",
      progress: 100,
    });

    return {
      ok: true,
      jobId,
      outputPath,
      durationMs: Date.now() - startedAt,
    };
  } catch (err) {
    updateJobProgress(jobId, {
      jobId,
      status: "failed",
      progress: 0,
      message: err instanceof Error ? err.message : String(err),
    });

    return {
      ok: false,
      jobId,
      error: err instanceof Error ? err.message : String(err),
      durationMs: Date.now() - startedAt,
    };
  }
}
