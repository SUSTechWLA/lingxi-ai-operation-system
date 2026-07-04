const fs = require('fs')
const path = require('path')

function createFileAccessController() {
  const grantedFiles = new Set()
  const grantedDirs = new Set()

  function grantFiles(filePaths = []) {
    for (const filePath of filePaths) {
      const real = realPath(filePath)
      if (real) grantedFiles.add(real)
    }
  }

  function grantDirectories(dirPaths = []) {
    for (const dirPath of dirPaths) {
      const real = realPath(dirPath)
      if (real) grantedDirs.add(withTrailingSeparator(real))
    }
  }

  function canRead(filePath) {
    const real = realPath(filePath)
    if (!real) return false
    if (grantedFiles.has(real)) return true
    const normalized = withTrailingSeparator(real)
    for (const dir of grantedDirs) {
      if (normalized.startsWith(dir)) return true
    }
    return false
  }

  return {
    grantFiles,
    grantDirectories,
    canRead,
  }
}

function realPath(value) {
  if (typeof value !== 'string' || !value.trim()) return ''
  try {
    return fs.realpathSync.native(path.resolve(value.trim()))
  } catch {
    return ''
  }
}

function withTrailingSeparator(value) {
  return value.endsWith(path.sep) ? value : `${value}${path.sep}`
}

module.exports = {
  createFileAccessController,
}
