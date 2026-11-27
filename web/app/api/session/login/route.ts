import { NextRequest } from "next/server";
import { beginLogin, sessionResponse } from "../../../server/session";

export async function POST(request: NextRequest) {
  try {
    return await beginLogin(request);
  } catch {
    return sessionResponse({ error: "Sign-in is temporarily unavailable." }, 503);
  }
}
