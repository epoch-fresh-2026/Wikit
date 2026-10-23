import { Component, Suspense, lazy, type ReactNode } from 'react'
import { Link, Navigate, Route, Routes, useLocation } from 'react-router'
import { SiteLayout, AdminLayout } from '@/components/layout'

import { Loading, SectionTitle } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/features/account/auth'
import { PageScreen } from '@/features/reading/screens'
const SearchScreen = lazy(() =>
  import('@/features/reading/search-screen').then((m) => ({ default: m.SearchScreen })),
)
const EditPageScreen = lazy(() =>
  import('@/features/editing/screens').then((m) => ({ default: m.EditPageScreen })),
)
const HistoryScreen = lazy(() =>
  import('@/features/reading/history').then((m) => ({ default: m.HistoryScreen })),
)
const RevisionScreen = lazy(() =>
  import('@/features/reading/history').then((m) => ({ default: m.RevisionScreen })),
)
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
      <div role="alert" className="error panel narrow">
        <h1>页面暂时无法加载</h1>
        <p className="muted my-4">请检查网络连接后重试。</p>
        <Button onClick={() => window.location.reload()}>重新加载页面</Button>
      </div>
    ) : (
      this.props.children
    )
  }
}

export function App() {
  const auth = useAuth(),
    location = useLocation()
  const Layout = location.pathname.startsWith('/admin') ? AdminLayout : SiteLayout
  return (
    <Layout>
      <div key={`${auth.epoch}:${location.pathname}`}>
        <PageBoundary>
          <Suspense fallback={<Loading />}>
            <Routes>
              <Route path="/" element={<PageScreen home />} />
              <Route path="/search" element={<SearchScreen />} />
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
              <Route path="/pages/:pageId/proposals/:id" element={<ProposalScreen />} />
              <Route path="/pages/:pageId/proposals/:id/edit" element={<ProposalScreen edit />} />
              <Route path="/pages/:id/issues" element={<IssueListScreen />} />
              <Route
                path="/pages/:pageId/issues/:id"
                element={
                  <IssueScreen
                    governance={new URLSearchParams(location.search).get('governance') === '1'}
                  />
                }
              />
              <Route path="/admin" element={<Navigate to="/admin/accounts" replace />} />
              <Route
                path="/admin/:section"
                element={
                  auth.account?.role === 'admin' ? (
                    <AdminScreen />
                  ) : (
                    <p role="alert">需要管理员权限。</p>
                  )
                }
              />
              <Route
                path="*"
                element={
                  <section className="narrow panel">
                    <SectionTitle
                      title="未找到此页面"
                      description="检查页面地址，或通过搜索继续寻找内容。"
                    />
                    <div className="actions">
                      <Button asChild>
                        <Link to="/search">搜索页面</Link>
                      </Button>
                      <Button variant="outline" asChild>
                        <Link to="/">返回首页</Link>
                      </Button>
                    </div>
                  </section>
                }
              />
            </Routes>
          </Suspense>
        </PageBoundary>
      </div>
    </Layout>
  )
}
