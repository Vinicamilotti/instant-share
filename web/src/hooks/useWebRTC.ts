import { useEffect, useRef, useState, useCallback } from "react";
import type { Guest, WSMessage, GuestJoinedPayload, AnswerPayload, IcePayload } from "../lib/types";
import { connectWS, sendWS } from "../lib/ws";

interface UseWebRTCOptions {
  sessionId: string;
  role: "host" | "guest";
  name?: string;
}

export function useWebRTC({ sessionId, role, name }: UseWebRTCOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const pcsRef = useRef<Map<string, RTCPeerConnection>>(new Map());
  const streamRef = useRef<MediaStream | null>(null);
  const [connected, setConnected] = useState(false);
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [guests, setGuests] = useState<Guest[]>([]);
  const [error, setError] = useState<string | null>(null);

  const rtcConfig: RTCConfiguration = {
    iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
  };

  const sendOfferToGuest = useCallback((guestId: string) => {
    if (!streamRef.current) {
      console.log("[host] sendOfferToGuest: no stream yet");
      return;
    }
    console.log("[host] sendOfferToGuest:", guestId, "tracks:", streamRef.current.getTracks().length);

    const pc = new RTCPeerConnection(rtcConfig);

    pc.oniceconnectionstatechange = () => {
      console.log(`[host] ICE state (guest ${guestId}):`, pc.iceConnectionState);
    };
    pcsRef.current.set(guestId, pc);

    pc.onicecandidate = (event) => {
      if (event.candidate) {
        console.log("[host] ICE candidate generated");
        if (wsRef.current) {
          sendWS(wsRef.current, {
            type: "ice-candidate",
            payload: {
              candidate: JSON.stringify(event.candidate),
              guest_id: guestId,
            },
          });
        }
      }
    };

    streamRef.current.getTracks().forEach((track) =>
      pc.addTrack(track, streamRef.current!)
    );

    pc.createOffer().then((offer) =>
      pc.setLocalDescription(offer).then(() => {
        if (wsRef.current) {
          sendWS(wsRef.current, {
            type: "offer",
            payload: {
              sdp: JSON.stringify(pc.localDescription),
              guest_id: guestId,
            },
          });
        }
      })
    );
  }, []);

  const startScreenShare = useCallback(async () => {
    try {
      const stream = await navigator.mediaDevices.getDisplayMedia({
        video: true,
        audio: true,
      });
      streamRef.current = stream;
      setLocalStream(stream);

      for (const g of guests) {
        sendOfferToGuest(g.id);
      }

      stream.getVideoTracks()[0].onended = () => {
        setLocalStream(null);
        streamRef.current = null;
        pcsRef.current.forEach((pc) => pc.close());
        pcsRef.current.clear();
      };
    } catch {
      setError("Failed to start screen share");
    }
  }, [guests, sendOfferToGuest]);

  const handleWSMessage = useCallback(
    (msg: WSMessage) => {
      switch (msg.type) {
        case "guest-list":
          setGuests(msg.payload.guests as Guest[]);
          break;

        case "guest-joined": {
          const p = msg.payload as unknown as GuestJoinedPayload;
          if (streamRef.current) {
            sendOfferToGuest(p.id);
          }
          break;
        }

        case "offer": {
          console.log("[guest] offer received");
          const pc = new RTCPeerConnection(rtcConfig);
          pcRef.current = pc;

          pc.oniceconnectionstatechange = () => {
            console.log("[guest] ICE state:", pc.iceConnectionState);
          };

          pc.onicecandidate = (event) => {
            if (event.candidate) {
              console.log("[guest] ICE candidate generated");
              if (wsRef.current) {
                sendWS(wsRef.current, {
                  type: "ice-candidate",
                  payload: { candidate: JSON.stringify(event.candidate) },
                });
              }
            }
          };

          pc.ontrack = (event) => {
            console.log("[guest] ontrack fired, streams:", event.streams.length);
            setRemoteStream(event.streams[0]);
          };

          const sdp = JSON.parse(msg.payload.sdp as string) as RTCSessionDescriptionInit;
          pc.setRemoteDescription(new RTCSessionDescription(sdp))
            .then(async () => {
              console.log("[guest] setRemoteDescription OK");
              const answer = await pc.createAnswer();
              await pc.setLocalDescription(answer);
              console.log("[guest] answer created, sending...");
              if (wsRef.current) {
                sendWS(wsRef.current, {
                  type: "answer",
                  payload: { sdp: JSON.stringify(pc.localDescription) },
                });
              }
            })
            .catch((err) => {
              console.error("[guest] setRemoteDescription failed:", err);
            });
          break;
        }

        case "answer": {
          console.log("[host] answer received for guest:", (msg.payload as unknown as AnswerPayload).guest_id);
          const p = msg.payload as unknown as AnswerPayload;
          const pc = pcsRef.current.get(p.guest_id);
          if (pc) {
            const sdp = JSON.parse(p.sdp) as RTCSessionDescriptionInit;
            pc.setRemoteDescription(new RTCSessionDescription(sdp));
          }
          break;
        }

        case "ice-candidate": {
          if (role === "host") {
            const p = msg.payload as unknown as IcePayload;
            const pc = pcsRef.current.get(p.guest_id);
            if (pc) {
              const candidate = JSON.parse(p.candidate) as RTCIceCandidateInit;
              pc.addIceCandidate(new RTCIceCandidate(candidate));
            }
          } else {
            const candidate = JSON.parse(msg.payload.candidate as string) as RTCIceCandidateInit;
            pcRef.current?.addIceCandidate(new RTCIceCandidate(candidate));
          }
          break;
        }

        case "kicked":
        case "session-ended":
          setError(
            msg.type === "kicked"
              ? "You were removed from the session"
              : "Host ended the session"
          );
          pcRef.current?.close();
          wsRef.current?.close();
          break;
      }
    },
    [sendOfferToGuest, role]
  );

  const kick = useCallback((guestId: string) => {
    const pc = pcsRef.current.get(guestId);
    if (pc) {
      pc.close();
      pcsRef.current.delete(guestId);
    }
    if (wsRef.current) {
      sendWS(wsRef.current, { type: "kick", payload: { guest_id: guestId } });
    }
  }, []);

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
      pcsRef.current.forEach((pc) => pc.close());
      pcsRef.current.clear();
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