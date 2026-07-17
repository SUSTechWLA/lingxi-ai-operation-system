/**
 * Minimal artifact-storage helpers.
 *
 * The AIOS local-backend already owns the canonical artifact store
 * (local-backend artifacts/ directory). This module provides convenience
 * helpers for the render service to write output files under whitelisted
 * roots, with path validation delegated to security.ts.
 */
import fs from "node:fs";
import path from "node:path";
import { assertPathAllowed, ensureDir } from "./security.js";
/**
 * Write a buffer to disk under an allowed output root and return artifact
 * metadata suitable for the AIOS artifact store.
 */
export function saveArtifact(filePath, data, kind, security) {
    const resolved = assertPathAllowed(filePath, security.allowedOutputRoots, "outputPath");
    ensureDir(path.dirname(resolved));
    fs.writeFileSync(resolved, data);
    const stat = fs.statSync(resolved);
    return {
        kind,
        name: path.basename(resolved),
        storageRef: `local://${path.relative(security.allowedOutputRoots[0] ?? "/tmp", resolved)}`,
        sizeBytes: stat.size,
    };
}
/**
 * Read a file from an allowed project root.
 */
export function readProjectFile(filePath, security) {
    const resolved = assertPathAllowed(filePath, security.allowedProjectRoots, "projectPath");
    return fs.readFileSync(resolved);
}
//# sourceMappingURL=storage.js.map