/**
 * Path-based security: only allow reads/writes within configured whitelist roots.
 * HyperFrames renders HTML in a browser context, so unrestricted filesystem access
 * is a risk that must be contained.
 */
import path from "node:path";
import fs from "node:fs";
/** Build a normalized allowlist from one required root and optional semicolon-separated roots. */
export function configuredRoots(primaryRoot, additionalRoots = "") {
    const roots = [primaryRoot, ...additionalRoots.split(";")]
        .map((root) => root.trim())
        .filter((root) => path.isAbsolute(root))
        .map((root) => path.resolve(root));
    return [...new Set(roots)];
}
/**
 * Resolves `targetPath` and asserts it falls within at least one of the
 * `allowedRoots`.  Throws if the resolved path escapes all allowed roots.
 * Returns the resolved absolute path on success so callers don't need a
 * second resolution.
 */
export function assertPathAllowed(targetPath, allowedRoots, label) {
    const resolved = path.resolve(targetPath);
    const allowed = allowedRoots.some((root) => {
        const resolvedRoot = path.resolve(root);
        return (resolved === resolvedRoot || resolved.startsWith(resolvedRoot + path.sep));
    });
    if (!allowed) {
        throw new Error(`${label} path is not within allowed roots: ${targetPath} (resolved: ${resolved})`);
    }
    return resolved;
}
/**
 * Ensure a directory exists (mkdir -p). Safe to call when it already exists.
 */
export function ensureDir(dirPath) {
    fs.mkdirSync(dirPath, { recursive: true });
}
//# sourceMappingURL=security.js.map