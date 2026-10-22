import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { api, ApiError, type Schema } from '@/lib/api'

type Auth = {
  account: Schema<'Account'> | null
  ready: boolean
  epoch: number
  login: (session: Schema<'Session'>) => void
  clear: () => void
}
const Context = createContext<Auth | null>(null)
export function AuthProvider({ children }: { children: ReactNode }) {
  const [account, setAccount] = useState<Schema<'Account'> | null>(null)
  const [ready, setReady] = useState(false)
  const [epoch, setEpoch] = useState(0)
  const [failure, setFailure] = useState<ApiError>()
  const identity = useRef<Schema<'Account'> | null>(null)
  const clear = () => {
    api.setCSRF('')
    setAccount(null)
    if (identity.current) {
      identity.current = null
      setEpoch((e) => e + 1)
    }
  }
  useEffect(() => {
    let active = true
    const controller = new AbortController()
    api.onUnauthorized = clear
    api.onSessionRestored = (session) => {
      if (!active) return false
      const sameIdentity = identity.current?.id === session.account.id
      identity.current = session.account
      setAccount(session.account)
      if (!sameIdentity) setEpoch((e) => e + 1)
      return sameIdentity
    }
    api
      .session(controller.signal)
      .then((value) => {
        if (active) {
          api.setCSRF(value.csrf_token)
          identity.current = value.account
          setAccount(value.account)
        }
      })
      .catch((error: ApiError) => {
        if (active) {
          if (error.code === 'unauthenticated') clear()
          else setFailure(error)
        }
      })
      .finally(() => {
        if (active) setReady(true)
      })
    return () => {
      active = false
      controller.abort()
      api.setCSRF('')
      api.onUnauthorized = () => {}
      api.onSessionRestored = () => true
    }
  }, [])
  if (!ready)
    return (
      <p className="boot" role="status">
        正在连接 Wikit…
      </p>
    )
  if (failure)
    return (
      <div className="boot" role="alert">
        <p>{failure.message}</p>
        <Button onClick={() => window.location.reload()}>重新连接</Button>
      </div>
    )
  return (
    <Context
      value={{
        account,
        ready,
        epoch,
        clear,
        login: (session) => {
          api.setCSRF(session.csrf_token)
          identity.current = session.account
          setAccount(session.account)
          setEpoch((e) => e + 1)
        },
      }}
    >
      {children}
    </Context>
  )
}
export function useAuth() {
  const context = useContext(Context)
  if (!context) throw new Error('AuthProvider missing')
  return context
}
