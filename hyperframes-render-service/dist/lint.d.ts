/**
 * Lint a HyperFrames project directory.
 * Validates the composition HTML, checks for non-deterministic patterns
 * (Date.now, Math.random), and reports issues.
 *
 * Phase 2: full validation. Phase 1: basic structure checks.
 */
import { type SecurityConfig } from "./security.js";
import type { LintRequest, LintResult } from "./types.js";
export declare function hasHTMLClass(content: string, className: string): boolean;
export declare function lintProject(req: LintRequest, security: SecurityConfig): Promise<LintResult>;
//# sourceMappingURL=lint.d.ts.map