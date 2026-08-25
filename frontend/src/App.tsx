import { useEffect, useState } from 'react'
import { Service as AppService } from '../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app'
import type { AppInfoDTO } from '../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app/models'

// This is Phase 2's placeholder shell: it proves the Go <-> Wails <-> React
// pipeline end to end (a real bound service call, not a demo) before Phase 3
// replaces this file with the real app shell (Vite + Tailwind + shadcn/ui +
// TanStack Query + i18next), per docs/migration/wails-v3-implementation-plan.md.
function App() {
  const [info, setInfo] = useState<AppInfoDTO | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    AppService.AppInfo()
      .then(setInfo)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))
  }, [])

  return (
    <main style={{ fontFamily: 'system-ui, sans-serif', padding: '2rem', color: '#1a1a1a' }}>
      <h1>Nestworth (Wails v3 shell)</h1>
      <p>
        Phase 2 placeholder: this page exists only to prove the Go backend,
        Wails IPC bindings, and React frontend are wired together correctly.
        Phase 3 replaces this with the real application shell.
      </p>
      {error && <p style={{ color: '#b91c1c' }}>AppInfo() failed: {error}</p>}
      {info && (
        <dl>
          <dt>Name</dt>
          <dd>{info.name}</dd>
          <dt>App ID</dt>
          <dd>{info.appId}</dd>
          <dt>Version</dt>
          <dd>{info.version} (build {info.build})</dd>
        </dl>
      )}
      {!info && !error && <p>Loading AppInfo() from Go...</p>}
    </main>
  )
}

export default App
