import type { HostSessionResponse } from "./types";

const BASE = "/api";

export async function createSession(name: string): Promise<HostSessionResponse> {
  const res = await fetch(`${BASE}/host-session`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw new Error("Failed to create session");
  return res.json();
}