"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ANALYTICS_API, api } from "./api";
import { AuthGuard } from "./auth";

type Summary = { total_clicks: number; unique_visitors: number };

export default function Home() {
  const [summary, setSummary] = useState<Summary>();
  useEffect(() => { api<{data: Summary}>(ANALYTICS_API, "/api/v1/analytics/summary").then(result => setSummary(result.data)).catch(() => undefined); }, []);
  return (
    <AuthGuard>
      <header className="page-header"><div><p className="eyebrow">Workspace overview</p><h1>Short links.<br/>Clear signals.</h1></div><Link className="button" href="/links">Create a link</Link></header>
      <section className="grid three">
        <article className="metric"><span>Total clicks</span><strong>{summary ? summary.total_clicks.toLocaleString() : "—"}</strong><small>Last 30 days</small></article>
        <article className="metric"><span>Unique visitors</span><strong>{summary ? summary.unique_visitors.toLocaleString() : "—"}</strong><small>Privacy-preserving daily IDs</small></article>
        <article className="metric accent"><span>Pipeline</span><strong>Live</strong><small>Redirects and analytics are decoupled</small></article>
      </section>
      <section className="panel">
        <p className="eyebrow">Getting started</p><h2>From long URL to useful signal</h2>
        <div className="steps"><div><b>01</b><span>Create a memorable alias</span></div><div><b>02</b><span>Share the short URL</span></div><div><b>03</b><span>Explore click analytics</span></div></div>
      </section>
    </AuthGuard>
  );
}
