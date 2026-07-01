/**
 * In-memory job queue and progress tracking.
 *
 * A lightweight job store so SSE clients and status polls can observe
 * render progress without touching the filesystem.
 */
import type { RenderProgress } from "./types.js";
export declare function updateJobProgress(jobId: string, progress: RenderProgress): void;
export declare function getJobProgress(jobId: string): RenderProgress | undefined;
/** List all job IDs (most-recent first). */
export declare function listJobIds(): string[];
/** Clean up jobs older than `maxAgeMs`. Called periodically. */
export declare function evictOldJobs(maxAgeMs?: number): void;
//# sourceMappingURL=queue.d.ts.map