import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";

import { configuredRoots } from "./security.js";

test("configuredRoots accepts primary and additional absolute roots", () => {
  const roots = configuredRoots("/data/aios/projects", "/Users/demo/App Data;/srv/render");
  assert.deepEqual(roots, [
    path.resolve("/data/aios/projects"),
    path.resolve("/Users/demo/App Data"),
    path.resolve("/srv/render"),
  ]);
});

test("configuredRoots removes duplicates and ignores relative additions", () => {
  const roots = configuredRoots("/data/aios/projects", "/data/aios/projects;relative/path;;");
  assert.deepEqual(roots, [path.resolve("/data/aios/projects")]);
});
