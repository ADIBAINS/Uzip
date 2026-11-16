"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, CONTROL_API } from "../api";
import { AuthGuard } from "../auth";

type LinkRecord = { id: string; alias: string; destination: string; redirect_type: number; created_at: string };
const REDIRECT_URL = process.env.NEXT_PUBLIC_REDIRECT_URL ?? "http://localhost:8081";

export default function Links() {
  const [links, setLinks] = useState<LinkRecord[]>([]);
  const [destination, setDestination] = useState("");
  const [alias, setAlias] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [copied, setCopied] = useState("");
  const [offset, setOffset] = useState(0);
  const load = useCallback(async () => {
    setLoading(true); setError("");
    try { const result = await api<{data: LinkRecord[]}>(CONTROL_API, `/api/v1/links?limit=20&offset=${offset}`); setLinks(result.data); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Could not load links"); }
    finally { setLoading(false); }
  }, [offset]);
  useEffect(() => { load(); }, [load]);

  async function submit(event: FormEvent) {
    event.preventDefault(); setError(""); setSubmitting(true);
    try {
      await api(CONTROL_API, "/api/v1/links", { method: "POST", body: JSON.stringify({ destination, alias, redirect_type: 302 }) });
      setDestination(""); setAlias(""); await load();
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Could not create link"); }
    finally { setSubmitting(false); }
  }
  async function copy(value: string) {
    try { await navigator.clipboard.writeText(value); }
    catch { setError("Copy failed. Select and copy the short URL manually."); return; }
    setCopied(value); window.setTimeout(() => setCopied(""), 1600);
  }

  return (
    <AuthGuard>
      <header className="page-header"><div><p className="eyebrow">Link management</p><h1>Your links</h1><p className="lede">Create, share and inspect every short URL in this workspace.</p></div></header>
      <section className="panel">
        <h2>Create a short link</h2>
        <form onSubmit={submit}><label>Destination URL<input type="url" placeholder="https://example.com/launch" value={destination} onChange={e => setDestination(e.target.value)} required /></label><label>Custom alias <small>optional</small><input pattern="(?:[a-zA-Z0-9_]|-){3,64}" maxLength={64} title="3–64 letters, numbers, hyphens or underscores" placeholder="launch" value={alias} onChange={e => setAlias(e.target.value)} /></label><button disabled={submitting}>{submitting ? "Creating…" : "Create link"}</button></form>
        {error && <p className="error" role="alert">{error}</p>}
      </section>
      <section className="panel"><div className="section-title"><h2>All links</h2><span>{links.length}</span></div>
        {loading ? <div className="skeleton-list"><i/><i/><i/></div> : links.length === 0 ? <p className="empty">No links yet. Create the first one above.</p> :
          <div className="list">{links.map(link => { const shortURL = `${REDIRECT_URL}/${link.alias}`; return <article key={link.id}><div className="link-details"><a href={shortURL} target="_blank" rel="noreferrer">{shortURL}</a><p title={link.destination}>{link.destination}</p></div><div className="row-actions"><small>{new Date(link.created_at).toLocaleDateString()}</small><button className="secondary-button" onClick={() => copy(shortURL)}>{copied === shortURL ? "Copied" : "Copy"}</button></div></article>})}</div>}
        <div className="pagination"><button className="secondary-button" disabled={loading || offset === 0} onClick={() => setOffset(v => Math.max(0, v - 20))}>Previous</button><span>Page {offset / 20 + 1}</span><button className="secondary-button" disabled={loading || links.length < 20} onClick={() => setOffset(v => v + 20)}>Next</button></div>
        <p role="status" aria-live="polite">{copied ? "Short URL copied to clipboard." : ""}</p>
      </section>
    </AuthGuard>
  );
}
