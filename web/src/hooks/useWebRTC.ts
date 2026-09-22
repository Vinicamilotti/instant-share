import { useEffect, useRef, useState, useCallback } from "react";
import type { Guest, WSMessage } from "../lib/types";
import { connectWS, sendWS } from "../lib/ws";

interface UseWebRTCOptions {
  sessionId: string;
  role: "host" | "guest";
  name?: string;
}

export function useWebRTC({ sessionId, role, name }: UseWebRTCOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const [connected, setConnected] = useState(false);
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [guests, setGuests] = useState<Guest[]>([]);
  const [error, setError] = useState<string | null>(null);

  const createPeerConnection = useCallback(() => {
    const config: RTCConfiguration = {
      iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
    };
    const pc = new RTCPeerConnection(config);
    pcRef.current = pc;

    pc.onicecandidate = (event) => {
      if (event.candidate && wsRef.current) {
        sendWS(wsRef.current, {
          type: "ice-candidate",
          payload: { candidate: JSON.stringify(event.candidate) },
        });
      }
    };

    pc.ontrack = (event) => {
      setRemoteStream(event.streams[0]);
    };

    return pc;
  }, []);

  const startScreenShare = useCallback(async () => {
    try {
      const stream = await navigator.mediaDevices.getDisplayMedia({
        video: true,
        audio: true,
      });
      setLocalStream(stream);

      const pc = createPeerConnection();
      stream.getTracks().forEach((track) => pc.addTrack(track, stream));

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);

      if (wsRef.current) {
        sendWS(wsRef.current, {
          type: "offer",
          payload: { sdp: JSON.stringify(pc.localDescription) },
        });
      }
    } catch (err) {
      setError("Failed to start screen share");
    }
  }, [createPeerConnection]);

  const handleWSMessage = useCallback(
    (msg: WSMessage) => {
      switch (msg.type) {
        case "guest-list":
          setGuests(msg.payload.guests as Guest[]);
          break;

        case "offer": {
          const pc = createPeerConnection();
          const sdp = JSON.parse(msg.payload.sdp as string) as RTCSessionDescriptionInit;
          pc.setRemoteDescription(new RTCSessionDescription(sdp)).then(async () => {
            const answer = await pc.createAnswer();
            await pc.setLocalDescription(answer);
            if (wsRef.current) {
              sendWS(wsRef.current, {
                type: "answer",
                payload: { sdp: JSON.stringify(pc.localDescription) },
              });
            }
          });
          break;
        }

        case "ice-candidate": {
          const candidate = JSON.parse(msg.payload.candidate as string) as RTCIceCandidateInit;
          pcRef.current?.addIceCandidate(new RTCIceCandidate(candidate));
          break;
        }

        case "kicked":
        case "session-ended":
          setError(msg.type === "kicked" ? "You were removed from the session" : "Host ended the session");
          pcRef.current?.close();
          wsRef.current?.close();
          break;
      }
    },
    [createPeerConnection]
  );

  const kick = useCallback(
    (guestId: string) => {
      if (wsRef.current) {
        sendWS(wsRef.current, { type: "kick", payload: { guest_id: guestId } });
      }
    },
    []
  );

  useEffect(() => {
    const ws = connectWS(sessionId, handleWSMessage);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      if (role === "host") {
        sendWS(ws, { type: "host-join", payload: {} });
      } else if (role === "guest" && name) {
        sendWS(ws, { type: "guest-join", payload: { name } });
      }
    };

    ws.onclose = () => setConnected(false);

    return () => {
      pcRef.current?.close();
      ws.close();
    };
  }, [sessionId, role, name, handleWSMessage]);

  return {
    connected,
    localStream,
    remoteStream,
    guests,
    error,
    startScreenShare,
    kick,
  };
}