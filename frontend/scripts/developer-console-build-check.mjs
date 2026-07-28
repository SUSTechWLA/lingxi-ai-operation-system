import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtemp, readFile, readdir, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const outputRoot = await mkdtemp(join(tmpdir(), 'developer-console-build-'))
const vite = new URL('../node_modules/vite/bin/vite.js', import.meta.url).pathname
const creatorShellSource = await readFile(new URL('../src/features/creator-studio/CreatorShell.tsx', import.meta.url), 'utf8')
const developerConsoleSource = await readFile(new URL('../src/features/developer-console/DeveloperConsolePage.tsx', import.meta.url), 'utf8')

assert.doesNotMatch(creatorShellSource, /developer-console|DeveloperConsolePage|#\/developer/)
assert.doesNotMatch(developerConsoleSource, /DirectorStudioPage/)
for (const [view, label] of [
  ['summary', '摘要'],
  ['timeline', '时间线'],
  ['tools', '工具与 MCP'],
  ['artifacts', '技术产物'],
  ['gates', '门禁'],
  ['recovery', '恢复'],
]) {
  assert.ok(
    developerConsoleSource.includes(`{ view: '${view}', label: '${label}' }`),
    `developer console must define the ${view} diagnostics tab`,
  )
}

try {
  for (const [name, consoleValue, enabled] of [['unset', undefined, false], ['disabled', '0', false], ['enabled', '1', true]]) {
    const outDir = join(outputRoot, name)
    const env = { ...process.env }
    if (consoleValue === undefined) delete env.VITE_ENABLE_DEVELOPER_CONSOLE
    else env.VITE_ENABLE_DEVELOPER_CONSOLE = consoleValue
    execFileSync(process.execPath, [vite, 'build', '--outDir', outDir, '--emptyOutDir', '--manifest'], {
      env,
      stdio: 'inherit',
    })
    const manifest = JSON.parse(await readFile(join(outDir, '.vite', 'manifest.json'), 'utf8'))
    const hasConsoleModule = Object.keys(manifest).some((key) => key.includes('DeveloperConsolePage'))
    assert.equal(hasConsoleModule, enabled, `${name} production build must ${enabled ? 'include' : 'exclude'} the console module`)
    const javascriptFiles = (await readdir(outDir, { recursive: true })).filter((file) => file.endsWith('.js'))
    const javascript = (await Promise.all(javascriptFiles.map((file) => readFile(join(outDir, file), 'utf8')))).join('\n')
    for (const diagnosticsUiString of ['开发诊断', '工具与 MCP 调用', '范围恢复与诊断包']) {
      assert.equal(
        javascript.includes(diagnosticsUiString),
        enabled,
        `${name} production build must ${enabled ? 'include' : 'exclude'} diagnostics UI string ${diagnosticsUiString}`,
      )
    }
  }
  console.log('developer console production build checks passed')
} finally {
  await rm(outputRoot, { recursive: true, force: true })
}
