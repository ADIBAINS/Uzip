import { NextRequest } from "next/server";
import { finishLogin } from "../../server/session";

export async function GET(request: NextRequest) {
  const response = await finishLogin(request);
  response.headers.set("Referrer-Policy", "no-referrer");
  return response;
}
