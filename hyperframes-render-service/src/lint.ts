/**
 * Lint a HyperFrames project directory.
 * Validates the composition HTML, checks for non-deterministic patterns
 * (Date.now, Math.random), and reports issues.
 *
 * Phase 2: full validation. Phase 1: basic structure checks.
 */

import { assertPathAllowed, type SecurityConfig } from "./security.js";
import type { LintRequest, LintResult, LintMessage } from "./types.js";
import fs from "node:fs";
import path from "node:path";

const NON_DETERMINISTIC_PATTERNS: { rule: string; pattern: RegExp }[] = [
  { rule: "no-date-now", pattern: /Date\.now\s*\(/ },
  { rule: "no-math-random", pattern: /Math\.random\s*\(/ },
  { rule: "no-fetch", pattern: /fetch\s*\(/ },
  { rule: "no-xml-http-request", pattern: /new\s+XMLHttpRequest\s*\(/ },
];

export function hasHTMLClass(content: string, className: string): boolean {
  for (const match of content.matchAll(/\bclass\s*=\s*(["'])(.*?)\1/gs)) {
    if (match[2].split(/\s+/).includes(className)) {
      return true;
    }
  }
  return false;
}

export async function lintProject(
  req: LintRequest,
  security: SecurityConfig,
): Promise<LintResult> {
  const startedAt = Date.now();
  const entry = req.entry ?? "index.html";
  const errors: LintMessage[] = [];
  const warnings: LintMessage[] = [];

  try {
    const projectDir = assertPathAllowed(
      req.projectDir,
      security.allowedProjectRoots,
      "projectDir",
    );

    // Check entry file exists.
    const entryPath = path.join(projectDir, entry);
    if (!fs.existsSync(entryPath)) {
      errors.push({
        rule: "entry-not-found",
        message: `Entry file not found: ${entry}`,
        file: entry,
      });
      return {
        ok: errors.length === 0,
        errors,
        warnings,
        entry,
        durationMs: Date.now() - startedAt,
      };
    }

    // Read entry and scan for non-deterministic patterns.
    const content = fs.readFileSync(entryPath, "utf-8");
    const lines = content.split("\n");

    for (const { rule, pattern } of NON_DETERMINISTIC_PATTERNS) {
      for (let i = 0; i < lines.length; i++) {
        if (pattern.test(lines[i])) {
          errors.push({
            rule,
            message: `Non-deterministic pattern "${rule}" found in ${entry}`,
            file: entry,
            line: i + 1,
          });
        }
      }
    }

    // Check for clip class requirement.
    if (!hasHTMLClass(content, "clip")) {
      warnings.push({
        rule: "no-clip-class",
        message:
          "No elements with class='clip' found. HyperFrames requires clip elements for timeline rendering.",
        file: entry,
      });
    }

    return {
      ok: errors.length === 0,
      errors,
      warnings,
      entry,
      durationMs: Date.now() - startedAt,
    };
  } catch (err) {
    errors.push({
      rule: "lint-error",
      message: err instanceof Error ? err.message : String(err),
    });
    return {
      ok: false,
      errors,
      warnings,
      entry,
      durationMs: Date.now() - startedAt,
    };
  }
}
