import {
  type DocumentNode, type SelectionSetNode, execute, getOperationAST, GraphQLError, Kind,
  NoSchemaIntrospectionCustomRule, parse, specifiedRules, validate, visit,
} from "graphql";
import { NextRequest } from "next/server";
import { resolvers, schema } from "../../server/graphql";
import { sameOrigin, sessionIdentity, sessionResponse } from "../../server/session";
import { demoMode } from "../../server/upstream";

const rules = [...specifiedRules, NoSchemaIntrospectionCustomRule];
const maxRootFields = 12;

function error(message: string, status: number) {
  return sessionResponse({ errors: [{ message }] }, status);
}

async function envelope(request: NextRequest) {
  const reader = request.body?.getReader();
  if (!reader) return undefined;
  const decoder = new TextDecoder("utf-8", { fatal: true });
  let length = 0;
  let raw = "";
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) return JSON.parse(raw + decoder.decode());
      length += value.length;
      if (length > 32 * 1024) break;
      raw += decoder.decode(value, { stream: true });
    }
  } finally {
    await reader.cancel().catch(() => undefined);
  }
  return undefined;
}

function countRootFields(document: DocumentNode, operationName?: string) {
  const operation = getOperationAST(document, operationName);
  if (!operation) return 0;
  const fragments = new Map(document.definitions.flatMap(node =>
    node.kind === Kind.FRAGMENT_DEFINITION ? [[node.name.value, node.selectionSet] as const] : []));
  const visited = new Set<string>();
  const fields = new Set<string>();
  function collect(selectionSet: SelectionSetNode) {
    for (const selection of selectionSet.selections) {
      if (selection.kind === Kind.FIELD) fields.add(selection.alias?.value ?? selection.name.value);
      if (selection.kind === Kind.INLINE_FRAGMENT) collect(selection.selectionSet);
      if (selection.kind === Kind.FRAGMENT_SPREAD && !visited.has(selection.name.value)) {
        visited.add(selection.name.value);
        const fragment = fragments.get(selection.name.value);
        if (fragment) collect(fragment);
      }
    }
  }
  collect(operation.selectionSet);
  return fields.size;
}

export async function POST(request: NextRequest) {
  const bearer = !demoMode ? /^Bearer (\S+)$/i.exec(request.headers.get("authorization") ?? "")?.[1] : undefined;
  if (!bearer && !sameOrigin(request)) return error("Invalid origin.", 403);
  if (request.headers.get("content-type")?.split(";")[0].trim().toLowerCase() !== "application/json") {
    return error("Expected an application/json request.", 415);
  }

  let identity: string | null;
  try {
    identity = bearer ?? await sessionIdentity(request);
  } catch {
    return error("Session is temporarily unavailable.", 503);
  }
  if (!identity) return error("Sign in required.", 401);

  let payload;
  try {
    payload = await envelope(request);
  } catch {
    return error("Invalid GraphQL request.", 400);
  }
  if (!payload || typeof payload.query !== "string" ||
    (payload.operationName != null && typeof payload.operationName !== "string") ||
    (payload.variables != null && (typeof payload.variables !== "object" || Array.isArray(payload.variables)))) {
    return error("Invalid GraphQL request.", 400);
  }

  try {
    const document = parse(payload.query, { maxTokens: 1200 });
    let fields = 0;
    visit(document, { Field: () => { fields++; } });
    if (fields > 80) return error("Too many fields in this document.", 400);
    const errors = validate(schema, document, rules);
    if (errors.length) return sessionResponse({ errors }, 400);
    if (countRootFields(document, payload.operationName) > maxRootFields) {
      return error("Too many root fields in this operation.", 400);
    }
    const result = await execute({
      schema, document, rootValue: resolvers, contextValue: { identity },
      variableValues: payload.variables, operationName: payload.operationName,
    });
    return sessionResponse(result);
  } catch (failure) {
    return failure instanceof GraphQLError ? sessionResponse({ errors: [failure] }, 400) : error("GraphQL request failed.", 500);
  }
}
