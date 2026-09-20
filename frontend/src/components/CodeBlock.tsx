// Docs code block with a clipboard copy button.
import { useState } from 'react'

function CodeBlock({ label = 'TERMINAL', code }: { label?: string; code: string }) {
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="docs-code">
      <div className="docs-code-head">
        <span>{label}</span>
        <button onClick={copy}>{copied ? 'Copied' : 'Copy'}</button>
      </div>
      <pre>
        <code>{code}</code>
      </pre>
    </div>
  )
}

export { CodeBlock }
