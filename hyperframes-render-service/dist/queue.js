/**
 * In-memory job queue and progress tracking.
 *
 * A lightweight job store so SSE clients and status polls can observe
 * render progress without touching the filesystem.
 */
const jobs = new Map();
export function updateJobProgress(jobId, progress) {
    jobs.set(jobId, progress);
}
export function getJobProgress(jobId) {
    return jobs.get(jobId);
}
/** List all job IDs (most-recent first). */
export function listJobIds() {
    return Array.from(jobs.keys()).reverse();
}
/** Clean up jobs older than `maxAgeMs`. Called periodically. */
export function evictOldJobs(maxAgeMs = 30 * 60 * 1000) {
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
//# sourceMappingURL=queue.js.map