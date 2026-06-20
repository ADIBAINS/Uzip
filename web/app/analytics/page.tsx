"use client";

import { useEffect, useState } from "react";
import { ANALYTICS_API, api } from "../api";
import { AuthGuard } from "../auth";

type Summary = { total_clicks: number; unique_visitors: number };
type Point = { timestamp: string; clicks: number; unique_visitors: number };

export default function Analytics() {
  const [summary, setSummary] = useState<Summary>();
  const [points, setPoints] = useState<Point[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    Promise.all([
      api<{data: Summary}>(ANALYTICS_API, "/api/v1/analytics/summary"),
      api<{data: Point[]}>(ANALYTICS_API, "/api/v1/analytics/timeseries?interval=day"),
    ]).then(([s, p]) => { setSummary(s.data); setPoints(p.data); }).catch(e => setError(e.message)).finally(() => setLoading(false));
  }, []);
  const max = Math.max(1, ...points.map(point => point.clicks));
  return (
    <AuthGuard>
      <header className="page-header"><div><p className="eyebrow">Last 30 days</p><h1>Analytics</h1><p className="lede">Human traffic only; identified bots are excluded.</p></div></header>
      {error && <p className="error" role="alert">{error}</p>}
      <section className="grid two"><article className="metric"><span>Total clicks</span><strong>{loading ? "—" : summary?.total_clicks.toLocaleString() ?? "0"}</strong></article><article className="metric"><span>Unique visitors</span><strong>{loading ? "—" : summary?.unique_visitors.toLocaleString() ?? "0"}</strong></article></section>
      <section className="panel"><div className="section-title"><div><p className="eyebrow">Daily volume</p><h2>Click activity</h2></div></div>
        {loading ? <div className="chart-skeleton"/> : points.length ? <div className="chart" role="img" aria-label="Daily clicks for the last 30 days">{points.map((point, index) => <div className="bar-column" key={point.timestamp}><span className="bar-value">{point.clicks}</span><i title={`${new Date(point.timestamp).toLocaleDateString()}: ${point.clicks} clicks`} style={{height: `${Math.max(5, point.clicks/max*100)}%`}}/><small>{index === 0 || index === points.length - 1 || index % 7 === 0 ? new Date(point.timestamp).toLocaleDateString(undefined, {month:"short", day:"numeric"}) : ""}</small></div>)}</div> : <p className="empty">Analytics will appear after links receive traffic.</p>}
      </section>
    </AuthGuard>
  );
}
