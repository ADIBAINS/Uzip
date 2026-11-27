"use client";

import { useEffect, useState } from "react";
import { graphql } from "../api";
import { AuthGuard } from "../auth";

type Entry = {
  id: string;
  kind: "created" | "creation_failed" | "click_milestone" | "expiring";
  alias: string;
  milestone?: number;
  expires_at?: string;
  created_at: string;
};

function message(entry: Entry) {
  switch (entry.kind) {
    case "created": return "Short link created";
    case "creation_failed": return "That alias was already taken";
    case "click_milestone": return `Reached ${entry.milestone?.toLocaleString()} human clicks`;
    case "expiring": return entry.expires_at ? `Expires ${new Date(entry.expires_at).toLocaleString()}` : "Link is expiring soon";
  }
}

export default function Notifications() {
  return <AuthGuard><Inbox /></AuthGuard>;
}

function Inbox() {
  const [entries, setEntries] = useState<Entry[]>();
  const [error, setError] = useState("");
  useEffect(() => {
    const request = new AbortController();
    graphql<{ notifications: Entry[] }>(`
      query Inbox { notifications { id kind alias milestone expires_at created_at } }
    `, {}, request.signal).then(result => {
      if (!request.signal.aborted) setEntries(result.notifications);
    }).catch(reason => {
      if (!request.signal.aborted) setError(reason.message);
    });
    return () => request.abort();
  }, []);
  return (
    <>
      <header className="page-header"><h1>Notifications</h1></header>
      <section className="panel">
        {error ? <p className="error" role="alert">{error}</p> :
          !entries ? <p className="empty" role="status">Loading notifications…</p> :
          entries.length === 0 ? <p className="empty">No notifications yet.</p> :
          <div className="list">{entries.map(entry => (
            <article key={entry.id}>
              <div className="link-details"><strong>{message(entry)}</strong><p>{entry.alias}</p></div>
              <time dateTime={entry.created_at}>{new Date(entry.created_at).toLocaleString()}</time>
            </article>
          ))}</div>}
      </section>
    </>
  );
}
