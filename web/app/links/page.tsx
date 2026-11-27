"use client";

import { FormEvent, useEffect, useState } from "react";
import { graphql } from "../api";
import { AuthGuard, useAuth } from "../auth";

type LinkRecord = { id: string; alias: string; destination: string; created_at: string; archived_at?: string };
const REDIRECT_URL = process.env.NEXT_PUBLIC_REDIRECT_URL ?? "http://localhost:8081";

export default function Links() {
  return <AuthGuard><LinksDashboard /></AuthGuard>;
}

function LinksDashboard() {
  const { claims } = useAuth();
  const role = claims?.["custom:role"];
  const canArchive = process.env.NEXT_PUBLIC_DEV_AUTH !== "false" || role === "owner" || role === "admin";
  const [links, setLinks] = useState<LinkRecord[]>([]);
  const [destination, setDestination] = useState("");
  const [alias, setAlias] = useState("");
  const [expiry, setExpiry] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [copied, setCopied] = useState("");
  const [offset, setOffset] = useState(0);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [listError, setListError] = useState("");
  const [archiving, setArchiving] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setListError("");
    graphql<{ links: LinkRecord[] }>(`
      query FindLinks($query: String, $offset: Int!) {
        links(query: $query, offset: $offset) { id alias destination created_at archived_at }
      }
    `, { query: search, offset }, controller.signal)
      .then(result => { if (!controller.signal.aborted) setLinks(result.links); })
      .catch(reason => {
        if (!controller.signal.aborted) setListError(reason instanceof Error ? reason.message : "Could not load links");
      })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [offset, search, refresh]);

  async function submit(event: FormEvent) {
    event.preventDefault(); setError(""); setSubmitting(true);
    try {
      await graphql(`
        mutation CreateLink($input: CreateLinkInput!) { createLink(input: $input) { short_url } }
      `, { input: { destination, alias, expires_at: expiry ? new Date(expiry).toISOString() : undefined } });
      setDestination("");
      setAlias("");
      setExpiry("");
      setSearchInput("");
      setSearch("");
      setOffset(0);
      setRefresh(value => value + 1);
    } catch (reason) { setError(reason instanceof Error ? reason.message : "Could not create link"); }
    finally { setSubmitting(false); }
  }
  async function copy(value: string) {
    try { await navigator.clipboard.writeText(value); }
    catch { setListError("Copy failed. Select and copy the short URL manually."); return; }
    setCopied(value); window.setTimeout(() => setCopied(""), 1600);
  }

  async function archive(link: LinkRecord) {
    if (!window.confirm(`Archive “${link.alias}”? Its short URL will stop redirecting.`)) return;
    setArchiving(link.id);
    setListError("");
    try {
      await graphql("mutation ArchiveLink($id: ID!) { archiveLink(id: $id) }", { id: link.id });
      setRefresh(value => value + 1);
    } catch (reason) {
      setListError(reason instanceof Error ? reason.message : "Could not archive link");
    } finally {
      setArchiving("");
    }
  }

  return (
    <>
      <header className="page-header"><div><p className="eyebrow">Link management</p><h1>Your links</h1><p className="lede">Create, share and inspect every short URL in this workspace.</p></div></header>
      <section className="panel">
        <h2>Create a short link</h2>
        <form className="link-create" onSubmit={submit}>
          <label>Destination URL<input type="url" placeholder="https://example.com/launch" value={destination} onChange={e => setDestination(e.target.value)} required /></label>
          <label><span>Custom alias <small>optional</small></span><input pattern="(?:[a-zA-Z0-9_]|-){3,64}" maxLength={64} title="3–64 letters, numbers, hyphens or underscores" placeholder="launch" value={alias} onChange={e => setAlias(e.target.value)} /></label>
          <label><span>Expires <small>optional</small></span><input type="datetime-local" value={expiry} onChange={event => setExpiry(event.target.value)} /></label>
          <button disabled={submitting}>{submitting ? "Creating…" : "Create link"}</button>
        </form>
        {error && <p className="error" role="alert">{error}</p>}
      </section>
      <section className="panel">
        <h2>All links</h2>
        <form className="link-search" onSubmit={event => {
          event.preventDefault();
          setOffset(0);
          setSearch(searchInput.trim());
          setRefresh(value => value + 1);
        }}>
          <label>Search links<input type="search" placeholder="Alias or destination URL" value={searchInput} onChange={event => setSearchInput(event.target.value)} /></label>
          <button>Search</button>
        </form>
        {listError && <p className="error" role="alert">{listError}</p>}
        {loading ? <div className="skeleton-list"><i/><i/><i/></div> : links.length === 0 ? (
          <p className="empty">{search ? "No links match your search." : "No links on this page."}</p>
        ) : (
          <div className="list">{links.map(link => {
            const shortURL = new URL(`${REDIRECT_URL}/${link.alias}`, window.location.origin).href;
            return <article key={link.id}>
              <div className="link-details">
                {link.archived_at ? <strong>{shortURL}</strong> : <a href={shortURL} target="_blank" rel="noreferrer">{shortURL}</a>}
                <p title={link.destination}>{link.destination}</p>
              </div>
              <div className="row-actions">
                <small>{new Date(link.created_at).toLocaleDateString()}</small>
                {link.archived_at ? <span>Archived</span> : <>
                  <button className="secondary-button" onClick={() => copy(shortURL)}>{copied === shortURL ? "Copied" : "Copy"}</button>
                  {canArchive && <button className="secondary-button" disabled={Boolean(archiving)} onClick={() => archive(link)}>{archiving === link.id ? "Archiving…" : "Archive"}</button>}
                </>}
              </div>
            </article>;
          })}</div>
        )}
        <div className="pagination"><button className="secondary-button" disabled={loading || offset === 0} onClick={() => setOffset(v => Math.max(0, v - 20))}>Previous</button><span>Page {offset / 20 + 1}</span><button className="secondary-button" disabled={loading || Boolean(listError) || links.length < 20} onClick={() => setOffset(v => v + 20)}>Next</button></div>
        <p role="status" aria-live="polite">{copied ? "Short URL copied to clipboard." : ""}</p>
      </section>
    </>
  );
}
