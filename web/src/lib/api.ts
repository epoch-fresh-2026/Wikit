import type { components } from './api.generated'
export type Schema<N extends keyof components['schemas']> = components['schemas'][N]

const messages: Record<string, string> = {
  invalid_request: '请求格式不正确，请检查填写内容。',
  unauthenticated: '登录已失效，请重新登录。',
  forbidden: '当前账户没有执行此操作的权限。',
  not_found: '内容不存在，或你已无权读取。',
  stale_revision: '版本已变化。你的草稿已保留，请对比最新内容后再提交。',
  merge_conflict: '修改存在冲突，请对比以下字段并在编辑器中解决。',
  invalid_state: '当前状态不允许此操作，请重新读取对象状态。',
  idempotency_conflict: '重试标识与请求内容不一致，请重新发起操作。',
  validation_failed: '内容未通过校验，请检查字段和长度。',
  rate_limited: '操作过于频繁，请稍后重试。',
  upload_too_large: '文件超出站点大小限制。',
  unsupported_media: '不支持此媒体类型。',
  search_unavailable: '搜索暂时不可用，仍可浏览页面。',
  storage_unavailable: '媒体存储暂时不可用。',
  internal_error: '服务暂时不可用，请保留草稿后重试。',
  network_error: '网络中断或请求超时，请保留草稿并重试。',
  authentication_failed: '验证未通过，请检查输入后重试。',
  session_changed: '登录账户已变化，请在当前账户下重新操作。',
  write_credentials_restored: '写入凭证已恢复，草稿已保留，请重新提交。',
}
export class ApiError extends Error {
  constructor(
    public code: string,
    public requestID = '',
    public conflicts?: Schema<'Error'>['error']['conflicts'],
  ) {
    super(messages[code] ?? messages.internal_error)
  }
}

// The CSRF nonce is not an authentication credential; the session stays HttpOnly.
function storedCSRF(value?: string): string {
  try {
    if (value !== undefined) {
      if (value) sessionStorage.setItem('wikit.csrf', value)
      else sessionStorage.removeItem('wikit.csrf')
    }
    return sessionStorage.getItem('wikit.csrf') ?? ''
  } catch {
    return value ?? ''
  }
}

export class Client {
  csrf = storedCSRF()
  onUnauthorized: () => void = () => {}
  onSessionRestored: (session: Schema<'Session'>) => boolean = () => true
  private generation = 0
  private recovery?: { generation: number; promise: Promise<void> }
  setCSRF(value: string) {
    this.generation++
    this.csrf = value
    storedCSRF(value)
  }

  // Startup owns cancellation and installation so an unmounted provider cannot
  // revive old credentials. This endpoint never mutates the session.
  session(signal?: AbortSignal) {
    return this.request<Schema<'Session'>>(
      '/api/v1/auth/session',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
        signal,
      },
      false,
    )
  }

  private restore(): Promise<void> {
    const generation = this.generation
    if (this.recovery?.generation === generation) return this.recovery.promise
    const promise = this.session()
      .then((session) => {
        if (this.generation !== generation) throw new ApiError('session_changed')
        this.setCSRF(session.csrf_token)
        if (!this.onSessionRestored(session)) throw new ApiError('session_changed')
      })
      .catch((error: unknown) => {
        if (error instanceof ApiError && error.code === 'unauthenticated') {
          if (this.generation !== generation) throw new ApiError('session_changed')
          this.setCSRF('')
          this.onUnauthorized()
        }
        throw error
      })
      .finally(() => {
        if (this.recovery?.promise === promise) this.recovery = undefined
      })
    this.recovery = { generation, promise }
    return promise
  }

  private async response(
    path: string,
    init: RequestInit = {},
    authenticatedWrite = true,
  ): Promise<Response> {
    if (!path.startsWith('/api/v1/') || path.includes('\\')) throw new ApiError('invalid_request')
    const headers = new Headers(init.headers)
    const write = !['GET', 'HEAD'].includes(init.method ?? 'GET')
    if (write && authenticatedWrite && !this.csrf) await this.restore()
    const generation = this.generation
    if (write && this.csrf) headers.set('X-CSRF-Token', this.csrf)
    let response: Response
    try {
      response = await fetch(path, {
        ...init,
        headers,
        credentials: 'same-origin',
        cache: 'no-store',
        redirect: 'error',
      })
    } catch (error) {
      if (init.signal?.aborted) throw error
      throw new ApiError('network_error')
    }
    if (!response.ok) {
      let error: Schema<'Error'>['error'] | undefined
      try {
        error = ((await response.json()) as Schema<'Error'>).error
      } catch {
        /* No server diagnostics are shown. */
      }
      if (authenticatedWrite && (response.status === 401 || (response.status === 403 && write))) {
        if (generation !== this.generation) throw new ApiError('session_changed')
        const previousCSRF = this.csrf
        await this.restore()
        // Never replay a mutation. A valid session means this 401 was an
        // operation's credential check, e.g. current password, not logout.
        if (response.status === 401) throw new ApiError('authentication_failed', error?.request_id)
        if (previousCSRF !== this.csrf)
          throw new ApiError('write_credentials_restored', error?.request_id)
      }
      throw new ApiError(
        error?.code ?? (response.status === 401 ? 'unauthenticated' : 'internal_error'),
        error?.request_id,
        error?.conflicts,
      )
    }
    return response
  }
  async request<T>(path: string, init: RequestInit = {}, authenticatedWrite = true): Promise<T> {
    const response = await this.response(path, init, authenticatedWrite)
    if (response.status === 204) return undefined as T
    return (await response.json()) as T
  }
  async media(path: string, signal: AbortSignal): Promise<Blob> {
    return (await this.response(path, { signal })).blob()
  }
  get<T>(path: string, signal?: AbortSignal) {
    return this.request<T>('/api/v1/' + path, { signal })
  }
  send<T>(path: string, body: unknown, method = 'POST', key?: string, authenticatedWrite = true) {
    return this.request<T>(
      '/api/v1/' + path,
      {
        method,
        headers: { 'Content-Type': 'application/json', ...(key ? { 'Idempotency-Key': key } : {}) },
        body: JSON.stringify(body),
      },
      authenticatedWrite,
    )
  }
  upload(path: string, file: File) {
    if (!/^\/api\/v1\/uploads\/[a-f\d-]{36}\/content$/.test(path))
      throw new ApiError('invalid_request')
    return this.request<Schema<'Blob'>>(path, {
      method: 'PUT',
      headers: { 'Content-Type': file.type },
      body: file,
    })
  }
}
export const api = new Client()
export function query(values: Record<string, string | null | undefined>) {
  const p = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) if (value) p.set(key, value)
  return p.toString()
}
