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
    <div className="rounded-2xl bg-primary-dark p-5 text-[#FFF7D8] shadow-card">
      <h4 className="text-sm font-semibold mb-3 flex items-center gap-2">
        <svg className="w-4 h-4 text-primary-light" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
              className="w-full px-3 py-2 rounded-lg border border-[#7A4A17] bg-[#1A0B02] text-sm font-mono text-[#FFF7D8] placeholder-[#B98A45] focus:outline-none focus:border-primary-light"
            />
          </div>
          <button
            onClick={runCommand}
            disabled={loading || !command.trim()}
            className="px-4 py-2 rounded-lg bg-primary text-white text-sm font-medium hover:bg-[#C7720D] disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
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
            className="flex-1 px-3 py-1.5 rounded-lg border border-[#7A4A17] bg-[#1A0B02] text-xs font-mono text-[#FFE9A8] placeholder-[#B98A45] focus:outline-none focus:border-primary-light"
          />
          <button
            onClick={pickDirectory}
            className="px-3 py-1.5 rounded-lg bg-[#7A4A17] text-[#FFF7D8] text-xs hover:bg-[#8B4A12] transition-colors"
          >
            选择目录
          </button>
        </div>

        {output && (
          <pre className="rounded-lg bg-[#120701] p-3 text-xs font-mono text-primary-light overflow-x-auto overflow-y-auto max-h-60 whitespace-pre-wrap">
            {output}
          </pre>
        )}
      </div>
    </div>
  )
}

export default CommandPanel
