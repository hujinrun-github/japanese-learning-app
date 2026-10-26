import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './styles/global.css'
import './i18n/config'
import App from './App'

// kuromojin resolves its dictionary path with require.resolve(), which does not exist in
// the browser bundle. Point it at the dictionary served from public/kuromoji-dict, which
// is fetched at deploy time by scripts/fetch-kuromoji-dict.mjs.
;(window as unknown as { kuromojin?: { dicPath: string } }).kuromojin = {
  dicPath: `${import.meta.env.BASE_URL}kuromoji-dict`,
}

const rootElement = document.getElementById('root')
if (!rootElement) {
  throw new Error('Root element #root not found in the document.')
}

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
