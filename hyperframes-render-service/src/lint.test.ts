import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { hasHTMLClass, lintProject } from "./lint.js";

test("hasHTMLClass recognizes a token among multiple classes", () => {
  assert.equal(hasHTMLClass('<video class="aroll-media clip"></video>', "clip"), true);
  assert.equal(hasHTMLClass("<video class='clip arroll-media'></video>", "clip"), true);
  assert.equal(hasHTMLClass('<video class="clipped"></video>', "clip"), false);
});

test("lintProject accepts generated clips with multiple class tokens", async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "hyperframes-lint-"));
  try {
    fs.writeFileSync(
      path.join(root, "index.html"),
      '<div data-composition><video class="aroll-media clip"></video></div>',
    );
    const result = await lintProject(
      { projectDir: root, entry: "index.html" },
      { allowedProjectRoots: [root], allowedOutputRoots: [root] },
    );
    assert.equal(result.ok, true);
    assert.deepEqual(result.warnings, []);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
