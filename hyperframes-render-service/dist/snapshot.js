/**
 * Keyframe snapshot generation.
 * Renders a HyperFrames project at specific timestamps and saves PNG frames.
 *
 * Phase 2: full Puppeteer-based snapshot. Phase 1: placeholder (returns empty).
 */
import { assertPathAllowed, ensureDir } from "./security.js";
import path from "node:path";
export async function takeSnapshots(req, security) {
    const projectDir = assertPathAllowed(req.projectDir, security.allowedProjectRoots, "projectDir");
    assertPathAllowed(req.outputDir, security.allowedOutputRoots, "outputDir");
    ensureDir(req.outputDir);
    const snapshots = [];
    // Phase 2: Use Puppeteer to render each frame at the requested times.
    // For now, return the expected snapshot paths as a placeholder so
    // downstream tools can reference them.
    for (const timeSec of req.times) {
        const filename = `${String(timeSec).padStart(4, "0")}.png`;
        const filePath = path.join(req.outputDir, filename);
        snapshots.push({ timeSec, path: filePath });
    }
    return { ok: true, snapshots };
}
//# sourceMappingURL=snapshot.js.map