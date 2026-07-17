/**
 * Keyframe snapshot generation.
 * Renders a HyperFrames project at specific timestamps and saves PNG frames.
 *
 * Phase 2: full Puppeteer-based snapshot. Phase 1: placeholder (returns empty).
 */
import { type SecurityConfig } from "./security.js";
import type { SnapshotRequest, SnapshotResult } from "./types.js";
export declare function takeSnapshots(req: SnapshotRequest, security: SecurityConfig): Promise<SnapshotResult>;
//# sourceMappingURL=snapshot.d.ts.map