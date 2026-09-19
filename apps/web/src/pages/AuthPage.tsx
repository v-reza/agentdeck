// Sign-in and registration forms. Presentation only; the API ships with M0.
import { Link } from 'react-router-dom'
import { Header } from '../components/Shell'

function AuthPage({ mode }: { mode: 'login' | 'register' }) {
  return (
    <>
      <Header />
      <main className="auth-page">
        <div className="auth-card">
          <div className="auth-mark">AD</div>
          <h1>{mode === 'login' ? 'Sign in to AgentDeck' : 'Create your AgentDeck workspace'}</h1>
          <p>{mode === 'login' ? 'Continue to your agent fleet.' : 'Start free. No credit card required.'}</p>
          <form onSubmit={(event) => event.preventDefault()}>
            <label>
              Email
              <input type="email" placeholder="you@company.com" />
            </label>
            {mode === 'register' && (
              <label>
                Workspace name
                <input type="text" placeholder="Production fleet" />
              </label>
            )}
            <label>
              Password
              <input type="password" placeholder="••••••••" />
            </label>
            <button className="btn-primary full" type="submit">
              {mode === 'login' ? 'Sign in' : 'Create workspace'}
            </button>
          </form>
          <div className="auth-foot">
            {mode === 'login' ? (
              <>
                New here? <Link to="/register">Create an account</Link>
              </>
            ) : (
              <>
                Already have an account? <Link to="/login">Sign in</Link>
              </>
            )}
          </div>
        </div>
      </main>
    </>
  )
}

export { AuthPage }
