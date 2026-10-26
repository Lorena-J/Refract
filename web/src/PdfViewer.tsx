import { useEffect, useLayoutEffect, useRef, useState, type MouseEvent } from 'react'
import * as pdfjs from 'pdfjs-dist'
import type { PDFDocumentLoadingTask, PDFDocumentProxy } from 'pdfjs-dist'
import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'

pdfjs.GlobalWorkerOptions.workerSrc = workerUrl

export type Highlight = { page: number; x: number; y: number; w: number; h: number; n: number }

type Props = {
  projectId: string
  version: number // bump to reload the PDF after a compile
  highlight?: Highlight
  onInverse: (page: number, x: number, y: number) => void
}

type Size = { w: number; h: number } // page size in PDF points

// Continuous-scroll viewer. Pages are laid out at their final size up front
// (so scroll position survives a reload) and rendered when they come into view.
export default function PdfViewer({ projectId, version, highlight, onInverse }: Props) {
  const [doc, setDoc] = useState<PDFDocumentProxy | null>(null)
  const [sizes, setSizes] = useState<Size[]>([])
  const [scale, setScale] = useState<number | 'fit'>('fit')
  const [current, setCurrent] = useState(1)
  const [message, setMessage] = useState('')
  const scroller = useRef<HTMLDivElement>(null)
  const keep = useRef<{ top: number; left: number } | null>(null)
  const zoomAnchor = useRef<number | null>(null) // relative scroll to keep across zoom changes
  const task = useRef<PDFDocumentLoadingTask | null>(null) // owns the shown document

  const fitScale = () => {
    const el = scroller.current
    return el && sizes[0] ? (el.clientWidth - 32) / sizes[0].w : 1
  }
  const s = scale === 'fit' ? fitScale() : scale

  // Load (or reload) the PDF.
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      const res = await fetch(`/api/projects/${projectId}/pdf`, { headers: { 'X-Refract': '1' } })
      if (!res.ok) {
        setMessage(res.status === 404 ? 'No PDF yet. Compile to see it here.' : `Could not load the PDF (HTTP ${res.status}).`)
        return
      }
      const data = new Uint8Array(await res.arrayBuffer())
      const loading = pdfjs.getDocument({ data })
      const loaded = await loading.promise
      const next: Size[] = []
      for (let i = 1; i <= loaded.numPages; i++) {
        const vp = (await loaded.getPage(i)).getViewport({ scale: 1 })
        next.push({ w: vp.width, h: vp.height })
      }
      if (cancelled) return loading.destroy()
      const el = scroller.current
      if (el) keep.current = { top: el.scrollTop, left: el.scrollLeft }
      setMessage('')
      setSizes(next)
      task.current?.destroy() // free the previous document
      task.current = loading
      setDoc(loaded)
    })().catch((e) => !cancelled && setMessage(`Could not load the PDF: ${e}`))
    return () => {
      cancelled = true
    }
  }, [projectId, version])

  useEffect(() => () => void task.current?.destroy(), [])

  // Restore the scroll position after a reload, once the new layout exists.
  useLayoutEffect(() => {
    const el = scroller.current
    if (el && keep.current) {
      el.scrollTop = keep.current.top
      el.scrollLeft = keep.current.left
      keep.current = null
    }
  }, [doc])

  // Keep the same relative position when zooming.
  useLayoutEffect(() => {
    const el = scroller.current
    if (el && zoomAnchor.current !== null) {
      el.scrollTop = zoomAnchor.current * el.scrollHeight
      zoomAnchor.current = null
    }
  }, [s])

  // Re-fit on resize.
  const [, force] = useState(0)
  useEffect(() => {
    const el = scroller.current
    if (!el) return
    const ro = new ResizeObserver(() => scale === 'fit' && force((n) => n + 1))
    ro.observe(el)
    return () => ro.disconnect()
  }, [scale])

  // Scroll to and flash a forward-sync highlight.
  useEffect(() => {
    const el = scroller.current
    const page = highlight && el?.querySelector<HTMLElement>(`[data-page="${highlight.page}"]`)
    if (!highlight || !el || !page) return
    // Instant, not smooth: a PDF reload landing mid-animation would otherwise
    // restore a half-way position.
    el.scrollTop = page.offsetTop + highlight.y * s - el.clientHeight / 3
  }, [highlight])

  function zoom(next: number | 'fit') {
    const el = scroller.current
    if (el) zoomAnchor.current = el.scrollTop / Math.max(el.scrollHeight, 1)
    setScale(next)
  }

  function onScroll() {
    const el = scroller.current
    if (!el) return
    const mid = el.scrollTop + el.clientHeight / 2
    for (const p of el.querySelectorAll<HTMLElement>('[data-page]')) {
      if (p.offsetTop + p.offsetHeight > mid) {
        setCurrent(Number(p.dataset.page))
        break
      }
    }
  }

  function goTo(n: number) {
    const p = scroller.current?.querySelector<HTMLElement>(`[data-page="${n}"]`)
    if (p && scroller.current) scroller.current.scrollTop = p.offsetTop - 8
  }

  return (
    <div className="pdf">
      <div className="pdf-bar">
        <button onClick={() => goTo(current - 1)} disabled={current <= 1} aria-label="Previous page">
          ‹
        </button>
        <input
          className="pdf-page"
          aria-label="Page"
          value={current}
          onChange={(e) => {
            const n = Number(e.target.value)
            if (n >= 1 && n <= sizes.length) goTo(n)
          }}
        />
        <span className="hint">/ {sizes.length}</span>
        <button onClick={() => goTo(current + 1)} disabled={current >= sizes.length} aria-label="Next page">
          ›
        </button>
        <span className="spacer" />
        <button onClick={() => zoom(Math.max(0.25, s / 1.2))} aria-label="Zoom out">
          −
        </button>
        <span className="hint">{Math.round(s * 100)}%</span>
        <button onClick={() => zoom(Math.min(5, s * 1.2))} aria-label="Zoom in">
          +
        </button>
        <button onClick={() => zoom('fit')} disabled={scale === 'fit'}>
          Fit
        </button>
      </div>
      <div className="pdf-scroll" ref={scroller} onScroll={onScroll}>
        {message && <p className="hint pdf-message">{message}</p>}
        {doc &&
          sizes.map((size, i) => (
            <Page
              key={i}
              doc={doc}
              n={i + 1}
              size={size}
              scale={s}
              highlight={highlight?.page === i + 1 ? highlight : undefined}
              onInverse={onInverse}
            />
          ))}
      </div>
    </div>
  )
}

function Page({ doc, n, size, scale, highlight, onInverse }: { doc: PDFDocumentProxy; n: number; size: Size; scale: number; highlight?: Highlight; onInverse: Props['onInverse'] }) {
  const box = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const [visible, setVisible] = useState(false)
  const [flash, setFlash] = useState<Highlight | undefined>()

  useEffect(() => {
    const io = new IntersectionObserver(([e]) => setVisible(e.isIntersecting), { rootMargin: '600px 0px' })
    io.observe(box.current!)
    return () => io.disconnect()
  }, [])

  useEffect(() => {
    if (!visible) return
    let cancelled = false
    let task: { cancel: () => void } | null = null
    ;(async () => {
      const page = await doc.getPage(n)
      if (cancelled || !canvas.current) return
      const dpr = window.devicePixelRatio || 1
      const vp = page.getViewport({ scale: scale * dpr })
      // Render off-screen, then swap: no blank flash on recompile or zoom.
      const off = document.createElement('canvas')
      off.width = Math.floor(vp.width)
      off.height = Math.floor(vp.height)
      const t = page.render({ canvas: off, viewport: vp })
      task = t
      await t.promise
      if (cancelled || !canvas.current) return
      canvas.current.width = off.width
      canvas.current.height = off.height
      canvas.current.getContext('2d')!.drawImage(off, 0, 0)
    })().catch(() => {}) // cancelled renders reject
    return () => {
      cancelled = true
      task?.cancel()
    }
  }, [doc, n, scale, visible])

  useEffect(() => {
    if (!highlight) return
    setFlash(highlight)
    const t = setTimeout(() => setFlash(undefined), 2000)
    return () => clearTimeout(t)
  }, [highlight])

  function onClick(e: MouseEvent) {
    if (!(e.metaKey || e.ctrlKey)) return
    const r = box.current!.getBoundingClientRect()
    onInverse(n, (e.clientX - r.left) / scale, (e.clientY - r.top) / scale)
  }

  return (
    <div className="pdf-page-box" data-page={n} ref={box} style={{ width: size.w * scale, height: size.h * scale }} onClick={onClick} title="Cmd/Ctrl+click: go to source">
      <canvas ref={canvas} style={{ width: '100%', height: '100%' }} />
      {flash && (
        <div
          className="sync-highlight"
          style={{ left: flash.x * scale - 2, top: flash.y * scale - 2, width: flash.w * scale + 4, height: flash.h * scale + 4 }}
        />
      )}
    </div>
  )
}
