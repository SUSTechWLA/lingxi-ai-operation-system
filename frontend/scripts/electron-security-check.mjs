import assert from 'node:assert/strict'
import { readdir, readFile, stat } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { join, relative } from 'node:path'

const root = fileURLToPath(new URL('..', import.meta.url))
const scanRoots = ['electron', 'src']
const extensions = new Set(['.cjs', '.js', '.jsx', '.mjs', '.ts', '.tsx'])

async function collectFiles(dir) {
  const entries = await readdir(dir)
  const out = []
  for (const entry of entries) {
    const path = join(dir, entry)
    const info = await stat(path)
    if (info.isDirectory()) {
      out.push(...await collectFiles(path))
      continue
    }
    const ext = entry.slice(entry.lastIndexOf('.'))
    if (extensions.has(ext)) out.push(path)
  }
  return out
}

const files = []
for (const dir of scanRoots) {
  files.push(...await collectFiles(join(root, dir)))
}

for (const filePath of files) {
  const file = relative(root, filePath)
  const content = await readFile(filePath, 'utf8')
  assert.equal(
    content.includes('execute-command') || content.includes('executeCommand'),
    false,
    `${file} must not expose arbitrary command execution in closed beta`
  )
  assert.equal(
    content.includes('publish-to-platforms') || content.includes('publishToPlatforms'),
    false,
    `${file} must not expose automatic platform publishing in closed beta`
  )
}

console.log(`electron security check passed for ${root}`)
