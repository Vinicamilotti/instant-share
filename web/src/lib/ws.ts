import type { WSMessage } from "./types";

export function connectWS(sessionId: string, onMessage: (msg: WSMessage) => void): WebSocket {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(`${protocol}//${location.host}/ws/${sessionId}`);

  ws.onmessage = (event) => {
    const msg: WSMessage = JSON.parse(event.data);
    onMessage(msg);
  };

  return ws;
}

export function sendWS(ws: WebSocket, msg: WSMessage): void {
  ws.send(JSON.stringify(msg));
}