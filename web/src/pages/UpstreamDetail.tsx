import { EmptyState, LoadingState } from '../Feedback'
import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, ApiError, type Upstream } from '../api'
import { source, useSources } from '../requests'
import { ErrorBox } from '../ui'
import { btn } from '../styles'
import UpstreamSecurity from '../invocation/UpstreamSecurity'
import UpstreamDetailForm from './UpstreamDetailForm'
import { UpstreamEditProvider } from './UpstreamEdit'

export default function UpstreamDetail({ create = false }: { create?: boolean }) {
  const { slug = '' } = useParams()
  const navigate = useNavigate()
  const [items, setItems] = useState<Upstream[]>([])
  const [replacement, setReplacement] = useState<Upstream | null>(null)
  const [revision, setRevision] = useState(0)
  const { load, loading, readError } = useSources(() =>
    create
      ? []
      : [
          source(
            'Upstream',
            async (signal) => {
              try {
                return await api.getUpstream(slug, signal)
              } catch (error) {
                if (error instanceof ApiError && error.status === 404) return undefined
                throw error
              }
            },
            (item) => setItems(item ? [item] : []),
          ),
        ],
  )
  useEffect(() => {
    void load()
  }, [load])
  const upstream =
    (replacement?.slug === slug ? replacement : null) ?? items.find((item) => item.slug === slug)
  if (!create && !upstream)
    return (
      <div className="page-layout">
        <h1 className="page-title">Upstream</h1>
        {loading ? (
          <LoadingState
            label="Loading saved configuration…"
            detail="Reading the saved upstream inventory."
          />
        ) : readError ? (
          <ErrorBox
            title="Upstream configuration is unavailable."
            onRetry={load}
            retryLabel="Retry"
            busy={loading}
          >
            {readError}
          </ErrorBox>
        ) : (
          <div className="surface-card">
            <EmptyState
              title={`Upstream “${slug}” was not found.`}
              description="Check the identifier or return to the upstream inventory."
            />
          </div>
        )}
        <div className="flex flex-wrap gap-3">
          <Link className={btn} to="/upstreams">
            Back to upstreams
          </Link>
          {!loading && !readError && (
            <button className={btn} onClick={() => void load()}>
              Retry
            </button>
          )}
        </div>
      </div>
    )

  const form = (
    <>
      {readError && (
        <ErrorBox title="Saved configuration could not be loaded" onRetry={load} busy={loading}>
          {readError}
        </ErrorBox>
      )}
      <UpstreamDetailForm
        key={`${slug}:${revision}`}
        upstream={create ? null : upstream!}
        onClose={() => navigate('/upstreams')}
        onReload={(latest) => {
          setReplacement(latest)
          setRevision((v) => v + 1)
        }}
        onSaved={(saved) => {
          setReplacement(saved)
          setRevision((v) => v + 1)
          if (create) navigate(`/upstreams/${encodeURIComponent(saved.slug)}`)
        }}
      />
    </>
  )
  return create ? (
    form
  ) : (
    <UpstreamEditProvider key={slug}>
      <div className="page-layout">
        <UpstreamSecurity slug={slug} version={upstream!.version}>
          {form}
        </UpstreamSecurity>
      </div>
    </UpstreamEditProvider>
  )
}
