/**
 * Core rendering logic using @hyperframes/producer.
 *
 * Delegates frame rendering + FFmpeg encoding to the HyperFrames producer
 * library so we never shell out to `npx hyperframes render`.
 */
import { type SecurityConfig } from "./security.js";
import type { RenderRequest, RenderResult } from "./types.js";
export declare function renderVideo(req: RenderRequest, security: SecurityConfig): Promise<RenderResult>;
//# sourceMappingURL=render.d.ts.map