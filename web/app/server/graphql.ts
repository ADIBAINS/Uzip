import { buildSchema, GraphQLError } from "graphql";
import { upstream } from "./upstream";

export const schema = buildSchema(`
  type Query {
    link(id: ID!): Link
    links(query: String, limit: Int = 20, offset: Int = 0): [Link!]!
    notifications: [Notification!]!
    analyticsSummary(linkId: ID, from: String, to: String): AnalyticsSummary!
    analyticsTimeseries(linkId: ID, interval: AnalyticsInterval = DAY, from: String, to: String): [AnalyticsPoint!]!
    analyticsBreakdown(linkId: ID, dimension: AnalyticsDimension!, from: String, to: String): [AnalyticsBreakdown!]!
  }
  type Mutation {
    createLink(input: CreateLinkInput!): CreatedLink!
    archiveLink(id: ID!): Boolean!
  }
  type Link {
    id: ID!
    alias: String!
    destination: String!
    redirect_type: Int!
    expires_at: String
    archived_at: String
    created_at: String!
  }
  input CreateLinkInput {
    destination: String!
    alias: String
    redirect_type: Int
    expires_at: String
  }
  type CreatedLink { data: Link!, short_url: String! }
  type Notification {
    id: ID!
    link_id: ID
    alias: String!
    kind: String!
    milestone: Int
    expires_at: String
    created_at: String!
  }
  enum AnalyticsInterval { HOUR DAY WEEK }
  enum AnalyticsDimension { COUNTRY DEVICE BROWSER OS REFERRER }
  type AnalyticsSummary { total_clicks: Float!, unique_visitors: Float! }
  type AnalyticsPoint { timestamp: String!, clicks: Float!, unique_visitors: Float! }
  type AnalyticsBreakdown { name: String!, clicks: Float! }
`);

type Args = Record<string, string | number | undefined | null>;
type Context = { identity: string };

function queryString(args: Args) {
  const values = new URLSearchParams();
  for (const [key, value] of Object.entries(args)) {
    if (value !== null && value !== undefined) values.set(key, String(value));
  }
  return values.size ? `?${values}` : "";
}

function linkPath(id: string) {
  if (!/^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$/i.test(id)) {
    throw new GraphQLError("Invalid link ID.", { extensions: { code: "BAD_USER_INPUT" } });
  }
  return `/api/v1/links/${id}`;
}

async function call<T>(ctx: Context, service: "control" | "analytics", path: string, init?: RequestInit, nullable = false): Promise<T> {
  let response: Response;
  try {
    response = await upstream(service, path, ctx.identity, init);
  } catch {
    throw new GraphQLError("The service is temporarily unavailable.", { extensions: { code: "UPSTREAM_UNAVAILABLE" } });
  }
  if (response.status === 404 && nullable) return { data: null } as T;
  if (!response.ok) {
    const data = await response.json().catch(() => undefined);
    throw new GraphQLError(data?.error?.message ?? "Request failed.", {
      extensions: {
        code: data?.error?.code ?? "UPSTREAM_ERROR",
        status: response.status,
        ...(response.headers.has("Retry-After") ? { retryAfter: response.headers.get("Retry-After") } : {}),
      },
    });
  }
  return response.status === 204 ? true as T : response.json();
}

async function data(ctx: Context, service: "control" | "analytics", path: string, nullable = false) {
  return (await call<{ data: unknown }>(ctx, service, path, undefined, nullable)).data;
}

export const resolvers = {
  link: ({ id }: { id: string }, ctx: Context) => data(ctx, "control", linkPath(id), true),
  links: (args: Args, ctx: Context) => data(ctx, "control", "/api/v1/links" + queryString(args)),
  notifications: (_args: Args, ctx: Context) => data(ctx, "control", "/api/v1/notifications"),
  analyticsSummary: ({ linkId, ...dates }: Args, ctx: Context) =>
    data(ctx, "analytics", "/api/v1/analytics/summary" + queryString({ ...dates, link_id: linkId })),
  analyticsTimeseries: ({ linkId, interval, ...dates }: Args, ctx: Context) =>
    data(ctx, "analytics", "/api/v1/analytics/timeseries" + queryString({ ...dates, link_id: linkId, interval: interval?.toString().toLowerCase() })),
  analyticsBreakdown: ({ linkId, dimension, ...dates }: { dimension: string } & Args, ctx: Context) =>
    data(ctx, "analytics", `/api/v1/analytics/breakdown/${dimension.toLowerCase()}` + queryString({ ...dates, link_id: linkId })),
  createLink: ({ input }: { input: object }, ctx: Context) =>
    call(ctx, "control", "/api/v1/links", {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input),
    }),
  archiveLink: ({ id }: { id: string }, ctx: Context) => call(ctx, "control", linkPath(id), { method: "DELETE" }),
};
