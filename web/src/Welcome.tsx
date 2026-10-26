import { api } from './api'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'

// First sign-in only (SIWC UI guidelines).
export default function Welcome({ open, manageUsage, onClose }: { open: boolean; manageUsage: string; onClose: () => void }) {
  async function close() {
    await api('/api/chatgpt/welcome-seen', { method: 'POST' }).catch(() => {})
    onClose()
  }
  return (
    <Dialog open={open} onOpenChange={(o) => !o && close()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>You're using your ChatGPT plan</DialogTitle>
          <DialogDescription>
            Eligible AI requests in Refract use your ChatGPT plan. You can review and manage this usage in{' '}
            <a className="underline underline-offset-4" href={manageUsage} target="_blank" rel="noreferrer">
              ChatGPT settings
            </a>
            .
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button autoFocus onClick={close}>
            Got it
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
