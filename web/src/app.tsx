import { Component, Suspense, lazy, useEffect, useRef, type ReactNode } from 'react'
import { Link, NavLink, Route, Routes, useLocation, useNavigate } from 'react-router'
import { BookOpen, Search, GitPullRequest, Plus, Settings } from 'lucide-react'
import { api } from '@/lib/api'
import { useAction } from '@/lib/hooks'
import { ActionFeedback, Loading } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/features/account/auth'
import {
  ExploreScreen,
  PageScreen,
  EditPageScreen,
  HistoryScreen,
  RevisionScreen,
} from '@/features/reading/screens'
const AccountScreen = lazy(() =>
  import('@/features/account/screens').then((m) => ({ default: m.AccountScreen })),
)
const LoginScreen = lazy(() =>
  import('@/features/account/screens').then((m) => ({ default: m.LoginScreen })),
)
const ProposalListScreen = lazy(() =>
  import('@/features/proposals/screens').then((m) => ({ default: m.ProposalListScreen })),
)
const ProposalScreen = lazy(() =>
  import('@/features/proposals/screens').then((m) => ({ default: m.ProposalScreen })),
)
const IssueListScreen = lazy(() =>
  import('@/features/discussions/screens').then((m) => ({ default: m.IssueListScreen })),
)
const IssueScreen = lazy(() =>
  import('@/features/discussions/screens').then((m) => ({ default: m.IssueScreen })),
)
const AdminScreen = lazy(() =>
  import('@/features/governance/screens').then((m) => ({ default: m.AdminScreen })),
)
const PolicyScreen = lazy(() =>
  import('@/features/governance/screens').then((m) => ({ default: m.PolicyScreen })),
)

class PageBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() {
    return { failed: true }
  }
  render() {
    return this.state.failed ? (
      <div role="alert" className="error">
        <p>页面暂时无法加载，请重新连接。</p>
        <Button onClick={() => window.location.reload()}>重新加载页面</Button>
      </div>
    ) : (
      this.props.children
    )
  }
}

export function App() {
  const auth = useAuth(),
    location = useLocation(),
    navigate = useNavigate(),
    action = useAction(),
    main = useRef<HTMLElement>(null)
  useEffect(() => {
    main.current?.focus()
    document.title = 'Wikit · 共建知识'
  }, [location.pathname])
  return (
    <div className="app">
      <a className="skip-link" href="#main">
        跳到主要内容
      </a>
      <header className="topbar">
        <Link className="brand" to="/" aria-label="Wikit 首页">
          <span className="brand-mark">
            <BookOpen size={23} strokeWidth={1.8} aria-hidden />
          </span>
          Wikit<span className="brand-tagline">共建知识</span>
        </Link>
        <nav aria-label="主导航">
          <NavLink to="/" end>
            <Search size={16} aria-hidden /> 探索
          </NavLink>
          <NavLink to="/proposals">
            <GitPullRequest size={16} aria-hidden /> 提案
          </NavLink>
        </nav>
        <div className="account-nav">
          {auth.account ? (
            <>
              <Link to="/account">{auth.account.display_name || auth.account.username}</Link>
              {!api.csrf && <Link to="/login">重新登录</Link>}
              {auth.account.role === 'admin' && (
                <Link to="/admin" aria-label="站点管理">
                  <Settings size={18} />
                </Link>
              )}
              <Button
                variant="ghost"
                disabled={action.busy}
                onClick={() =>
                  void action.run(async () => {
                    await api.send('auth/logout', {})
                    auth.clear()
                    navigate('/')
                  })
                }
              >
                退出
              </Button>
            </>
          ) : (
            <>
              <Link to="/login">登录</Link>
              <Link className="button-link secondary" to="/register">
                加入 Wikit
              </Link>
            </>
          )}
        </div>
      </header>
      <main id="main" ref={main} tabIndex={-1} className="main">
        <ActionFeedback action={action} />
        <div key={`${auth.epoch}:${location.pathname}:${location.search}`}>
          <PageBoundary>
            <Suspense fallback={<Loading />}>
              <Routes>
                <Route path="/" element={<ExploreScreen />} />
                <Route path="/login" element={<LoginScreen />} />
                <Route path="/register" element={<LoginScreen register />} />
                <Route path="/account" element={<AccountScreen />} />
                <Route path="/pages/new" element={<EditPageScreen create />} />
                <Route path="/pages/:id" element={<PageScreen />} />
                <Route path="/pages/:id/edit" element={<EditPageScreen />} />
                <Route path="/pages/:id/history" element={<HistoryScreen />} />
                <Route path="/pages/:id/revisions/:revisionID" element={<RevisionScreen />} />
                <Route path="/pages/:id/policy" element={<PolicyScreen />} />
                <Route path="/pages/:id/proposals" element={<ProposalListScreen />} />
                <Route path="/proposals" element={<ProposalListScreen />} />
                <Route path="/proposals/:id" element={<ProposalScreen />} />
                <Route path="/pages/:id/issues" element={<IssueListScreen />} />
                <Route
                  path="/issues/:id"
                  element={
                    <IssueScreen
                      governance={new URLSearchParams(location.search).get('governance') === '1'}
                    />
                  }
                />
                <Route path="/admin" element={<AdminScreen />} />
                <Route
                  path="*"
                  element={
                    <section>
                      <h1>未找到此页面</h1>
                      <Link to="/">返回首页</Link>
                    </section>
                  }
                />
              </Routes>
            </Suspense>
          </PageBoundary>
        </div>
      </main>
      <footer className="footer">
        <span>Wikit · 在协作中，让知识更好。</span>
        <Link to="/pages/new">
          <Plus size={14} aria-hidden /> 贡献一页知识
        </Link>
      </footer>
    </div>
  )
}
