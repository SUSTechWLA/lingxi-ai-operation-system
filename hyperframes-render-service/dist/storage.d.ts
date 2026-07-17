/**
 * Minimal artifact-storage helpers.
 *
 * The AIOS local-backend already owns the canonical artifact store
 * (local-backend artifacts/ directory). This module provides convenience
 * helpers for the render service to write output files under whitelisted
 * roots, with path validation delegated to security.ts.
 */
import { type SecurityConfig } from "./security.js";
export interface ArtifactMeta {
    kind: "VIDEO" | "SNAPSHOT" | "HTML" | "JSON";
    name: string;
    storageRef: string;
    sizeBytes: number;
}
/**
 * Write a buffer to disk under an allowed output root and return artifact
 * metadata suitable for the AIOS artifact store.
 */
export declare function saveArtifact(filePath: string, data: Buffer, kind: ArtifactMeta["kind"], security: SecurityConfig): ArtifactMeta;
/**
 * Read a file from an allowed project root.
 */
export declare function readProjectFile(filePath: string, security: SecurityConfig): Buffer;
//# sourceMappingURL=storage.d.ts.map