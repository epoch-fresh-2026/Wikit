import { useEffect, useRef, useState } from 'react'
import { api, ApiError } from './api'

export function useResource<T>(path: string | null, revision = 0) {
  const key = `${path}:${revision}`
  const [state, setState] = useState<{ key: string; data?: T; error?: ApiError }>({ key })
  useEffect(() => {
    if (!path) return
    const controller = new AbortController()
    let active = true
    setState({ key })
    api
      .get<T>(path, controller.signal)
      .then((data) => {
        if (active) setState({ key, data })
      })
      .catch((error) => {
        if (active)
          setState({
            key,
            error: error instanceof ApiError ? error : new ApiError('internal_error'),
          })
      })
    return () => {
      active = false
      controller.abort()
    }
  }, [key, path])
  return state.key === key ? state : { key }
}

export function useAction() {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<ApiError>()
  const [notice, setNotice] = useState('')
  const inFlight = useRef(false)
  const retry = useRef<{ signature: string; key: string } | undefined>(undefined)
  async function write<T>(path: string, body: unknown, method = 'POST'): Promise<T> {
    const signature = `${method}:${path}:${JSON.stringify(body)}`
    if (retry.current?.signature !== signature)
      retry.current = { signature, key: crypto.randomUUID() }
    const out = await api.send<T>(path, body, method, retry.current.key)
    retry.current = undefined
    return out
  }
  async function run(task: () => Promise<void>) {
    if (inFlight.current) return
    inFlight.current = true
    setBusy(true)
    setError(undefined)
    setNotice('')
    try {
      await task()
    } catch (e) {
      setError(e instanceof ApiError ? e : new ApiError('internal_error'))
    } finally {
      inFlight.current = false
      setBusy(false)
    }
  }
  return { busy, error, notice, setNotice, run, write }
}
