const assert = require('node:assert/strict')
const { mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } = require('node:fs')
const { tmpdir } = require('node:os')
const path = require('node:path')
const test = require('node:test')

const { createFileAccessController } = require('./file-access-runtime.cjs')

test('read access is denied until the exact file is granted', () => {
  const root = mkdtempSync(path.join(tmpdir(), 'tangying-file-access-'))
  try {
    const file = path.join(root, 'demo.txt')
    writeFileSync(file, 'demo')
    const access = createFileAccessController()

    assert.equal(access.canRead(file), false)
    access.grantFiles([file])
    assert.equal(access.canRead(file), true)
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})

test('granting a directory does not grant arbitrary files outside it through symlinks', () => {
  const root = mkdtempSync(path.join(tmpdir(), 'tangying-file-access-'))
  const outside = mkdtempSync(path.join(tmpdir(), 'tangying-file-outside-'))
  try {
    const allowedDir = path.join(root, 'allowed')
    mkdirSync(allowedDir)
    const outsideFile = path.join(outside, 'secret.txt')
    writeFileSync(outsideFile, 'secret')
    const link = path.join(allowedDir, 'secret-link.txt')
    symlinkSync(outsideFile, link)

    const access = createFileAccessController()
    access.grantDirectories([allowedDir])

    assert.equal(access.canRead(link), false)
  } finally {
    rmSync(root, { recursive: true, force: true })
    rmSync(outside, { recursive: true, force: true })
  }
})
