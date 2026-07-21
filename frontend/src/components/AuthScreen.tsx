import React, { useState } from 'react'
import { FiKey, FiLogIn, FiMail, FiRefreshCw, FiUser } from 'react-icons/fi'
import { login, register, type AuthUser } from '../services/auth'
import { APP_ICON_PATH } from '../utils/brand'

interface AuthScreenProps {
  onAuthenticated: (user: AuthUser) => void
}

const AuthScreen: React.FC<AuthScreenProps> = ({ onAuthenticated }) => {
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [nickname, setNickname] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setLoading(true)
    setError('')
    try {
      const session = mode === 'login'
        ? await login(email, password)
        : await register(email, password, nickname)
      onAuthenticated(session.user)
    } catch (err) {
      setError(err instanceof Error ? err.message : '认证失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-background px-6 py-8 text-ink">
      <div className="mx-auto flex min-h-[calc(100vh-4rem)] max-w-5xl items-center justify-center">
        <div className="grid w-full overflow-hidden rounded-lg border border-line bg-background-card shadow-card md:grid-cols-[0.9fr_1.1fr]">
          <div className="hidden bg-brand-panel p-8 text-on-brand-panel md:flex md:flex-col md:justify-between">
            <div>
              <div className="flex items-center gap-3">
                <img src={APP_ICON_PATH} alt="躺营 AI" className="h-9 w-9 rounded-lg" />
                <div>
                  <div className="text-base font-semibold">躺营 AI</div>
                  <div className="text-xs text-on-brand-panel/60">AI 视频创作助手</div>
                </div>
              </div>
              <div className="mt-12 border-l border-primary-light pl-5">
                <p className="text-sm font-medium text-primary-light">Cloud identity</p>
                <p className="mt-3 max-w-xs text-sm leading-6 text-on-brand-panel/75">
                  登录后项目、素材索引和运行记录按账号隔离；模型 API Key 仅在桌面端持久化，执行请求遵循设置页的传输说明。
                </p>
              </div>
            </div>
            <div className="text-xs leading-5 text-on-brand-panel/55">Provider secrets persist only on this device.</div>
          </div>

          <form onSubmit={submit} className="p-6 sm:p-8">
            <div className="md:hidden mb-8 flex items-center gap-3">
              <img src={APP_ICON_PATH} alt="躺营 AI" className="h-9 w-9 rounded-lg" />
              <div>
                <div className="text-base font-semibold">躺营 AI</div>
                <div className="text-xs text-ink-soft">AI 视频创作助手</div>
              </div>
            </div>

            <div className="flex rounded-lg border border-line bg-background p-1">
              <button
                type="button"
                onClick={() => setMode('login')}
                className={`h-9 flex-1 rounded-md text-sm font-medium ${mode === 'login' ? 'bg-background-card text-ink shadow-sm' : 'text-ink-soft'}`}
              >
                登录
              </button>
              <button
                type="button"
                onClick={() => setMode('register')}
                className={`h-9 flex-1 rounded-md text-sm font-medium ${mode === 'register' ? 'bg-background-card text-ink shadow-sm' : 'text-ink-soft'}`}
              >
                注册
              </button>
            </div>

            <h1 className="mt-8 text-2xl font-semibold">{mode === 'login' ? '登录账号' : '创建账号'}</h1>
            <p className="mt-2 text-sm text-ink-muted">使用邮箱和密码进入创作台。</p>

            <div className="mt-8 space-y-4">
              {mode === 'register' && (
                <Field icon={<FiUser />} label="昵称">
                  <input
                    value={nickname}
                    onChange={(event) => setNickname(event.target.value)}
                    className="h-11 w-full bg-transparent text-sm outline-none"
                    placeholder="可选"
                  />
                </Field>
              )}
              <Field icon={<FiMail />} label="邮箱">
                <input
                  type="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  className="h-11 w-full bg-transparent text-sm outline-none"
                  placeholder="name@example.com"
                  autoComplete="email"
                  required
                />
              </Field>
              <Field icon={<FiKey />} label="密码">
                <input
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  className="h-11 w-full bg-transparent text-sm outline-none"
                  placeholder="至少 8 位，包含字母和数字"
                  autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
                  required
                />
              </Field>
            </div>

            {error && (
              <div className="mt-4 rounded-lg border border-danger-line bg-danger-soft px-3 py-2 text-sm text-danger-ink">
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={loading}
              className="mt-6 flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-primary text-sm font-semibold text-on-primary shadow-glow transition hover:bg-primary-dark hover:text-on-primary-dark disabled:opacity-60"
            >
              {loading ? <FiRefreshCw className="h-4 w-4 animate-spin" /> : <FiLogIn className="h-4 w-4" />}
              {mode === 'login' ? '登录' : '注册并登录'}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}

const Field: React.FC<{ icon: React.ReactNode, label: string, children: React.ReactNode }> = ({ icon, label, children }) => (
  <label className="block">
    <span className="text-xs font-medium text-ink-muted">{label}</span>
    <span className="mt-1 flex items-center gap-3 rounded-lg border border-line bg-background-card px-3 focus-within:border-primary">
      <span className="text-ink-soft">{icon}</span>
      {children}
    </span>
  </label>
)

export default AuthScreen
