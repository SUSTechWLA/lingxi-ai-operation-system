/**
 * In-memory job queue and progress tracking.
 *
 * A lightweight job store so SSE clients and status polls can observe
 * render progress without touching the filesystem.
 */

import type { RenderProgress } from "./types.js";

const jobs = new Map<string, RenderProgress>();

export function updateJobProgress(
  jobId: string,
  progress: RenderProgress,
): void {
  jobs.set(jobId, progress);
}

export function getJobProgress(jobId: string): RenderProgress | undefined {
  return jobs.get(jobId);
}

/** List all job IDs (most-recent first). */
export function listJobIds(): string[] {
  return Array.from(jobs.keys()).reverse();
}

/** Clean up jobs older than `maxAgeMs`. Called periodically. */
export function evictOldJobs(maxAgeMs: number = 30 * 60 * 1000): void {
  // In-memory map — we don't track creation time explicitly, so this is a
  // best-effort placeholder. A real implementation would stamp each entry.
  if (jobs.size > 1000) {
    // Drop oldest half when over threshold.
    const keys = Array.from(jobs.keys());
    for (let i = 0; i < keys.length / 2; i++) {
      jobs.delete(keys[i]);
    }
  }
}
