import React, { useState } from 'react'
import { getElectronAPI } from '../utils/electron'

const CommandPanel: React.FC = () => {
  const [command, setCommand] = useState('')
  const [workDir, setWorkDir] = useState('/tmp/tangying-sandbox')
  const [output, setOutput] = useState('')
  const [loading, setLoading] = useState(false)

  const api = getElectronAPI()

  const runCommand = async () => {
    if (!api || !command.trim()) return

    setLoading(true)
    setOutput('')
    try {
      const parts = command.trim().split(/\s+/)
      const cmd = parts[0]
      const args = parts.slice(1)
      const result = await api.executeCommand(cmd, args, workDir)
      setOutput(
        (result.stdout ? `[stdout]\n${result.stdout}\n` : '') +
        (result.stderr ? `[stderr]\n${result.stderr}\n` : '') +
        `\n[exit code: ${result.exitCode}]`
      )
    } catch (e: any) {
      setOutput(`错误：${e.message}`)
    } finally {
      setLoading(false)
    }
  }

  const pickDirectory = async () => {
    if (!api) return
    try {
      const paths = await api.openDirectoryDialog()
      if (paths && paths.length > 0) {
        setWorkDir(paths[0])
      }
    } catch {
      // user cancelled
    }
  }

  return (
    <div className="bg-gray-900 rounded-2xl p-5 text-gray-100">
      <h4 className="text-sm font-semibold mb-3 flex items-center gap-2">
        <svg className="w-4 h-4 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
        </svg>
        命令执行
      </h4>

      <div className="space-y-3">
        <div className="flex gap-2">
          <div className="flex-1">
            <input
              type="text"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && runCommand()}
              placeholder="输入命令，如：ls -la"
              className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-sm font-mono text-gray-100 placeholder-gray-500 focus:outline-none focus:border-green-500"
            />
          </div>
          <button
            onClick={runCommand}
            disabled={loading || !command.trim()}
            className="px-4 py-2 bg-green-600 text-white rounded-lg text-sm font-medium hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? '执行中...' : '执行'}
          </button>
        </div>

        <div className="flex gap-2">
          <input
            type="text"
            value={workDir}
            onChange={(e) => setWorkDir(e.target.value)}
            placeholder="工作目录"
            className="flex-1 px-3 py-1.5 bg-gray-800 border border-gray-700 rounded-lg text-xs font-mono text-gray-300 placeholder-gray-500 focus:outline-none focus:border-green-500"
          />
          <button
            onClick={pickDirectory}
            className="px-3 py-1.5 bg-gray-700 text-gray-300 rounded-lg text-xs hover:bg-gray-600 transition-colors"
          >
            选择目录
          </button>
        </div>

        {output && (
          <pre className="bg-gray-950 rounded-lg p-3 text-xs font-mono text-green-300 overflow-x-auto overflow-y-auto max-h-60 whitespace-pre-wrap">
            {output}
          </pre>
        )}
      </div>
    </div>
  )
}

export default CommandPanel
