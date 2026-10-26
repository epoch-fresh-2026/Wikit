import { Link, useSearchParams } from 'react-router'
import { type Schema, query } from '@/lib/api'
import { Collection, DateText, SectionTitle, TagLinks } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/features/account/auth'
import { SearchFilters } from './search-filters'

export function SearchScreen() {
  const auth = useAuth()
  const [params, setParams] = useSearchParams()
  const search = (params.get('q') ?? '').trim()
  const type = params.get('type') ?? ''
  const filtered =
    params.getAll('tags').length > 0 ||
    !!params.get('tag') ||
    params.getAll('exclude_tags').length > 0
  const request = new URLSearchParams(params)
  request.delete('q')
  request.delete('cursor')
  if (search) request.set('q', search)
  const path = (search ? 'search?' : 'pages?') + request.toString()
  return (
    <section className="search-screen">
      <SectionTitle
        title={search ? '搜索结果' : '搜索页面'}
        description={search ? `与「${search}」相关的页面` : '通过关键词或标签，找到需要的知识。'}
      />
      <SearchFilters key={params.toString()} params={params} apply={(value) => setParams(value)} />
      {search ? (
        <Collection<Schema<'SearchHit'>>
          key={path}
          path={path}
          empty={
            <>
              未找到匹配页面。
              {!filtered && !type && auth.account?.can_create_page && (
                <Button variant="link" asChild>
                  <Link to={`/pages/new?${query({ title: search })}`}>创建页面</Link>
                </Button>
              )}
            </>
          }
        >
          {(hit) => (
            <article className="directory-row" key={hit.page_id}>
              <h2>
                <Link to={'/pages/' + hit.page_id}>{hit.title}</Link>
              </h2>
              {hit.passages?.length ? (
                hit.passages.map((passage, index) => (
                  <div key={`${passage.start_line}-${index}`}>
                    <Link to={passage.citation_url}>
                      {passage.section_path.length ? passage.section_path.join(' › ') : '正文'}
                      {` · 第 ${passage.start_line}–${passage.end_line} 行 · 固定版本`}
                    </Link>
                    <p>
                      {passage.text}
                      {passage.truncated ? '…' : ''}
                    </p>
                  </div>
                ))
              ) : hit.snippet ? (
                <p>{hit.snippet}</p>
              ) : (
                <p className="muted">页面匹配，暂无可定位的正文摘录。</p>
              )}
              <TagLinks tags={hit.tags} />
            </article>
          )}
        </Collection>
      ) : (
        <Collection<Schema<'PageDetail'>> key={path} path={path}>
          {(item) => (
            <article className="directory-row" key={item.page.id}>
              <h2>
                <Link to={'/pages/' + item.page.id}>{item.revision?.snapshot.title}</Link>
              </h2>
              <p className="muted">
                {item.page.type === 'document' ? '文档' : '媒体'} · 页面 {item.page.id}
              </p>
              {item.revision && (
                <>
                  <TagLinks tags={item.revision.snapshot.tags} />
                  <small>
                    <DateText value={item.revision.created_at} />
                  </small>
                </>
              )}
            </article>
          )}
        </Collection>
      )}
    </section>
  )
}
