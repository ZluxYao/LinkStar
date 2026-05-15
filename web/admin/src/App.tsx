import { useEffect, useState } from 'react'
import { AppShell } from './layout/AppShell'
import { Dashboard } from './pages/Dashboard'
import { Ddns } from './pages/Ddns'
import { Placeholder } from './pages/Placeholder'
import { Stun } from './pages/Stun'
import { findNav } from './layout/nav'
import type { PageKey } from './types'

function readPageFromHash(): PageKey {
  const h = window.location.hash.replace(/^#\/?/, '') as PageKey
  return findNav(h) ? h : 'dashboard'
}

function App() {
  const [page, setPage] = useState<PageKey>(() => readPageFromHash())

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
