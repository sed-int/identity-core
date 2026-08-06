import { useEffect, useState, type FormEvent } from 'react'
import { ApiError, createPost, listPosts, type Post } from '../api'
import { clearTokens } from '../auth'

interface Props {
  onLogout: () => void
}

export default function Board({ onLogout }: Props) {
  const [posts, setPosts] = useState<Post[]>([])
  const [nextPageAfter, setNextPageAfter] = useState('0') // "0" = no more pages
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  function fail(err: unknown) {
    // A 401 here means refresh already failed (api.ts retried once) — log out.
    if (err instanceof ApiError && err.status === 401) {
      clearTokens()
      onLogout()
    } else {
      setError(String(err))
    }
  }

  async function load(pageAfter?: string) {
    try {
      const res = await listPosts(pageAfter)
      setPosts((prev) => (pageAfter ? [...prev, ...(res.posts ?? [])] : (res.posts ?? [])))
      setNextPageAfter(res.nextPageAfter ?? '0')
    } catch (err) {
      fail(err)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await createPost(title, content)
      setTitle('')
      setContent('')
      await load() // reload first page so the new post shows on top
    } catch (err) {
      fail(err)
    } finally {
      setBusy(false)
    }
  }

  function logout() {
    clearTokens()
    onLogout()
  }

  return (
    <main>
      <div className="card row">
        <h1>Board</h1>
        <button onClick={logout}>Logout</button>
      </div>
      <form className="card" onSubmit={submit}>
        <input
          placeholder="Title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          required
        />
        <textarea
          placeholder="Content"
          value={content}
          onChange={(e) => setContent(e.target.value)}
          rows={3}
          required
        />
        <button disabled={busy}>Post</button>
      </form>
      {error && <p className="error">{error}</p>}
      {posts.map((p) => (
        <article className="card" key={p.id}>
          <h2>{p.title}</h2>
          <p>{p.content}</p>
          <p className="post-meta">
            {p.authorNickname || p.authorId} · {new Date(p.createdAt).toLocaleString()}
          </p>
        </article>
      ))}
      {nextPageAfter !== '0' && (
        <button onClick={() => load(nextPageAfter)}>Load more</button>
      )}
    </main>
  )
}
