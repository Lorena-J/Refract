import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import Login from './Login'
import './index.css'

const root = createRoot(document.getElementById('root')!)
// Signed out (REFRACT_AUTH=password): the API answers 401 and the sign-in page shows instead.
fetch('/api/me', { headers: { 'X-Refract': '1' } })
  .then((r) => r.status === 401, () => false)
  .then((signedOut) =>
    root.render(
      <StrictMode>{signedOut ? <Login /> : <App />}</StrictMode>,
    ),
  )
