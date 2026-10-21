import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router'
import {
  BookOpen,
  Files,
  GitPullRequest,
  Plus,
  Search,
  Settings,
  Users,
  Shield,
  ScrollText,
} from 'lucide-react'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from '@/components/ui/sidebar'
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

function Navigation({ admin }: { admin: boolean }) {
  const auth = useAuth(),
    location = useLocation(),
    sidebar = useSidebar()
  const items = admin
    ? ([
        ['/admin/accounts', '账户', Users],
        ['/admin/groups', '用户组', Shield],
        ['/admin/audit', '审计', ScrollText],
      ] as const)
    : ([
        ['/', '首页', BookOpen],
        ['/pages', '全部页面', Files],
        ['/proposals', '全站提案', GitPullRequest],
      ] as const)
  useEffect(() => {
    sidebar.setOpenMobile(false)
  }, [location.pathname])
  return (
    <Sidebar>
      <SidebarHeader>
        <Link to={admin ? '/admin/accounts' : '/'} className="site-name">
          <BookOpen size={20} />
          {admin ? 'Wikit 管理' : 'Wikit'}
        </Link>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {items.map(([to, label, Icon]) => (
                <SidebarMenuItem key={to}>
                  <SidebarMenuButton asChild isActive={location.pathname === to}>
                    <NavLink to={to} end>
                      <Icon />
                      <span>{label}</span>
                    </NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
              {!admin && auth.account?.can_create_page && (
                <SidebarMenuItem>
                  <SidebarMenuButton asChild>
                    <Link to="/pages/new">
                      <Plus />
                      创建页面
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild>
              <Link to={admin ? '/' : '/account'}>
                <Settings />
                {admin ? '返回站点' : '账户'}
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  )
}

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
    <SidebarProvider>
      <a className="skip-link" href="#main">
        跳到主要内容
      </a>
      <Navigation admin={admin} />
      <SidebarInset>
        <header className="site-header">
          <SidebarTrigger aria-label="切换导航" />
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
          <div className="ml-auto flex items-center gap-2">
            {auth.account && !api.csrf && <Link to="/login">重新登录</Link>}
            {auth.account ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost">
                    {auth.account.display_name || auth.account.username}
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
        <main
          id="main"
          tabIndex={-1}
          ref={main}
          className={admin ? 'workspace admin-workspace' : 'workspace'}
        >
          <ActionFeedback action={action} />
          {children}
        </main>
      </SidebarInset>
    </SidebarProvider>
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
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink asChild>
              <Link to="/pages">页面</Link>
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbLink asChild>
              <Link to={`/pages/${id}`}>{pageTitle || `页面 ${id}`}</Link>
            </BreadcrumbLink>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>
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
