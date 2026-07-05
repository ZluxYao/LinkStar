import { useEffect, useState } from 'react'
import { AppShell } from './layout/AppShell'
import { Dashboard } from './pages/Dashboard'
import { Ddns } from './pages/Ddns'
import { Placeholder } from './pages/Placeholder'
import { Stun } from './pages/Stun'
import { Login, Setup } from './pages/Auth'
import { findNav } from './layout/nav'
import { getAuthStatus, getToken, isDesktop } from './lib/api'
import type { PageKey } from './types'

function readPageFromHash(): PageKey {
  const h = window.location.hash.replace(/^#\/?/, '') as PageKey
  return findNav(h) ? h : 'dashboard'
}

type AuthState = 'loading' | 'setup' | 'login' | 'authed'

function App() {
  const [page, setPage] = useState<PageKey>(() => readPageFromHash())
  const [auth, setAuth] = useState<AuthState>('loading')

  // 启动：查初始化状态，决定进引导 / 登录 / 后台
  useEffect(() => {
    let alive = true
    getAuthStatus()
      .then((s) => {
        if (!alive) return
        if (!s.initialized) setAuth('setup')
        else if (isDesktop() || getToken()) setAuth('authed')
        else setAuth('login')
      })
      .catch(() => {
        if (alive) setAuth('login')
      })
    return () => {
      alive = false
    }
  }, [])

  useEffect(() => {
    const onHash = () => setPage(readPageFromHash())
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  const navigate = (key: PageKey) => {
    if (window.location.hash !== `#/${key}`) {
      window.location.hash = `#/${key}`
    }
    setPage(key)
  }

  if (auth === 'loading') {
    return <div className="grid min-h-screen place-items-center bg-slate-50 text-sm text-slate-400">加载中…</div>
  }
  if (auth === 'setup') {
    return <Setup onSuccess={() => setAuth('authed')} />
  }
  if (auth === 'login') {
    return <Login onSuccess={() => setAuth('authed')} />
  }

  const view = (() => {
    switch (page) {
      case 'dashboard':
        return <Dashboard />
      case 'stun':
        return <Stun />
      case 'ddns':
        return <Ddns />
      default: {
        const item = findNav(page)
        return <Placeholder name={item?.label ?? page} />
      }
    }
  })()

  return (
    <AppShell active={page} onChange={navigate}>
      {view}
    </AppShell>
  )
}

export default App
