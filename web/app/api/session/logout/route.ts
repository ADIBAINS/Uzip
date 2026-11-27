import { NextRequest } from "next/server";
import { endSession, sessionResponse } from "../../../server/session";

export async function POST(request: NextRequest) {
  try {
    return await endSession(request);
  } catch {
    return sessionResponse({ error: "Sign-out is temporarily unavailable. Try again." }, 503);
  }
}
