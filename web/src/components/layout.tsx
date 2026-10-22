import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router'
import { BookOpen, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Breadcrumb,
  BreadcrumbList,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { useAuth } from '@/features/account/auth'
import { api, type Schema } from '@/lib/api'
import { useAction, useResource } from '@/lib/hooks'
import { ActionFeedback } from '@/components/shared'

export function SiteLayout({ children, admin = false }: { children: ReactNode; admin?: boolean }) {
  const auth = useAuth(),
    navigate = useNavigate(),
    location = useLocation(),
    action = useAction()
  const [query, setQuery] = useState(''),
    main = useRef<HTMLElement>(null)
  useEffect(() => {
    main.current?.focus()
  }, [location.pathname])
  useEffect(() => {
    setQuery(new URLSearchParams(location.search).get('q') ?? '')
  }, [location.search])
  return (
    <div className="site-shell">
      <a className="skip-link" href="#main">
        跳到主要内容
      </a>
      <div className="min-w-0">
        <header className="site-header" aria-label="站点">
          <Link to="/" className="site-name" aria-label="Wikit 首页">
            <BookOpen size={20} />
            <span>Wikit</span>
          </Link>
          {admin ? (
            <span className="font-medium">站点管理</span>
          ) : (
            <form
              role="search"
              className="site-search"
              onSubmit={(e) => {
                e.preventDefault()
                navigate('/search?q=' + encodeURIComponent(query.trim()))
              }}
            >
              <Input
                aria-label="搜索页面"
                placeholder="搜索页面"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
              <Button variant="ghost" size="icon" aria-label="搜索">
                <Search />
              </Button>
            </form>
          )}
          <div className="site-account">
            {auth.account && !api.csrf && <Link to="/login">重新登录</Link>}
            {auth.account ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    className="account-menu"
                    title={auth.account.display_name || auth.account.username}
                  >
                    <span className="truncate">
                      {auth.account.display_name || auth.account.username}
                    </span>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem asChild>
                    <Link to="/account">账户设置</Link>
                  </DropdownMenuItem>
                  {auth.account.role === 'admin' && (
                    <DropdownMenuItem asChild>
                      <Link to="/admin/accounts">站点管理</Link>
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem
                    disabled={action.busy}
                    onSelect={() =>
                      void action.run(async () => {
                        await api.send('auth/logout', {})
                        auth.clear()
                        navigate('/')
                      })
                    }
                  >
                    退出
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : (
              <>
                <Button variant="ghost" asChild>
                  <Link to="/login">登录</Link>
                </Button>
                <Button variant="outline" asChild>
                  <Link to="/register">注册</Link>
                </Button>
              </>
            )}
          </div>
        </header>
        {admin && (
          <nav className="admin-navigation" aria-label="管理导航">
            {[
              ['/admin/accounts', '账户'],
              ['/admin/groups', '用户组'],
              ['/admin/audit', '审计'],
              ['/', '返回站点'],
            ].map(([to, label]) => (
              <Button key={to} variant="ghost" size="sm" asChild>
                <NavLink to={to} end>
                  {label}
                </NavLink>
              </Button>
            ))}
          </nav>
        )}
        <main
          id="main"
          tabIndex={-1}
          ref={main}
          className={admin ? 'workspace admin-workspace' : 'workspace'}
        >
          <ActionFeedback action={action} />
          {children}
        </main>
      </div>
    </div>
  )
}

export function PageNavigation({
  id,
  title,
  capabilities,
}: {
  id: string
  title?: string
  capabilities?: string[]
}) {
  const context = useResource<Schema<'PageDetail'>>(!title || !capabilities ? `pages/${id}` : null)
  const caps = capabilities ?? context.data?.capabilities ?? []
  const pageTitle = title ?? context.data?.revision?.snapshot.title
  return (
    <>
      {id !== '1' && (
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link to="/">首页</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem className="min-w-0 max-w-full">
              <BreadcrumbLink asChild className="min-w-0 [overflow-wrap:anywhere]">
                <Link to={`/pages/${id}`}>{pageTitle || `页面 ${id}`}</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      )}
      <nav className="page-tools" aria-label="页面操作">
        {[
          ['', '阅读'],
          ['/issues', '讨论'],
          ['/history', '历史'],
          ['/proposals', '提案'],
        ].map(([suffix, label]) => (
          <Button key={suffix} variant="ghost" size="sm" asChild>
            <NavLink end={!suffix} to={`/pages/${id}${suffix}`}>
              {label}
            </NavLink>
          </Button>
        ))}
        {caps.includes('propose') && (
          <Button size="sm" variant="outline" asChild>
            <Link to={`/pages/${id}/edit`}>提出修改</Link>
          </Button>
        )}
        {caps.includes('manage') && (
          <Button size="sm" variant="ghost" asChild>
            <Link to={`/pages/${id}/policy`}>页面管理</Link>
          </Button>
        )}
      </nav>
    </>
  )
}

export function AdminLayout({ children }: { children: ReactNode }) {
  return <SiteLayout admin>{children}</SiteLayout>
}
