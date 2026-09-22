export interface Guest {
  id: string;
  name: string;
}

export interface WSMessage {
  type: string;
  payload: Record<string, unknown>;
}

export interface HostSessionResponse {
  session_id: string;
  link: string;
}