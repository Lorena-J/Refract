import { api } from './api'

// First sign-in only (SIWC UI guidelines).
export default function Welcome({ manageUsage, onClose }: { manageUsage: string; onClose: () => void }) {
  async function close() {
    await api('/api/chatgpt/welcome-seen', { method: 'POST' }).catch(() => {})
    onClose()
  }
  return (
    <div className="modal-backdrop">
      <div className="modal" role="dialog" aria-modal="true" aria-labelledby="welcome-title">
        <h2 id="welcome-title">You're using your ChatGPT plan</h2>
        <p>
          Eligible AI requests in Refract use your ChatGPT plan. You can review and manage this usage in{' '}
          <a href={manageUsage} target="_blank" rel="noreferrer">
            ChatGPT settings
          </a>
          .
        </p>
        <button autoFocus onClick={close}>
          Got it
        </button>
      </div>
    </div>
  )
}
