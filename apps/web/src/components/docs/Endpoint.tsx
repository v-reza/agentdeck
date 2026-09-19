// REST endpoint card used by the API reference.
import { CodeBlock } from '../CodeBlock'

function Endpoint({
  method,
  path,
  role,
  description,
  code,
}: {
  method: string
  path: string
  role: string
  description: string
  code: string
}) {
  return (
    <div className="endpoint">
      <div className="endpoint-head">
        <span className={`method method-${method.toLowerCase()}`}>{method}</span>
        <code>{path}</code>
        <span className="endpoint-role">min role: {role}</span>
      </div>
      <p>{description}</p>
      <CodeBlock label="JSON" code={code} />
    </div>
  )
}

export { Endpoint }
