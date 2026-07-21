/**
 * Path-based security: only allow reads/writes within configured whitelist roots.
 * HyperFrames renders HTML in a browser context, so unrestricted filesystem access
 * is a risk that must be contained.
 */
export interface SecurityConfig {
    allowedProjectRoots: string[];
    allowedOutputRoots: string[];
}
/** Build a normalized allowlist from one required root and optional semicolon-separated roots. */
export declare function configuredRoots(primaryRoot: string, additionalRoots?: string): string[];
/**
 * Resolves `targetPath` and asserts it falls within at least one of the
 * `allowedRoots`.  Throws if the resolved path escapes all allowed roots.
 * Returns the resolved absolute path on success so callers don't need a
 * second resolution.
 */
export declare function assertPathAllowed(targetPath: string, allowedRoots: string[], label: string): string;
/**
 * Ensure a directory exists (mkdir -p). Safe to call when it already exists.
 */
export declare function ensureDir(dirPath: string): void;
//# sourceMappingURL=security.d.ts.map