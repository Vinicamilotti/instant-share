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
  const streamRef = useRef<MediaStream | null>(null);
  const [connected, setConnected] = useState(false);
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [guests, setGuests] = useState<Guest[]>([]);
  const [error, setError] = useState<string | null>(null);

  const rtcConfig: RTCConfiguration = {};

  const startScreenShare = useCallback(async () => {
    try {
      const stream = await navigator.mediaDevices.getDisplayMedia({
        video: true,
      });
      streamRef.current = stream;
      setLocalStream(stream);

      const pc = new RTCPeerConnection(rtcConfig);
      pcRef.current = pc;

      pc.oniceconnectionstatechange = () => {
        console.log("[host] ICE state:", pc.iceConnectionState);
      };

      pc.onicecandidate = (event) => {
        if (event.candidate) {
          console.log("[host] ICE candidate generated");
          if (wsRef.current) {
            sendWS(wsRef.current, {
              type: "host-ice-candidate",
              payload: {
                candidate: JSON.stringify(event.candidate),
              },
            });
          }
        }
      };

      stream.getTracks().forEach((track) => {
        pc.addTrack(track, stream);
      });

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);
      console.log("[host] host-offer SDP:", pc.localDescription?.sdp.substring(0, 200));

      if (wsRef.current) {
        sendWS(wsRef.current, {
          type: "host-offer",
          payload: {
            sdp: JSON.stringify(pc.localDescription),
          },
        });
      }

      stream.getVideoTracks()[0].onended = () => {
        setLocalStream(null);
        streamRef.current = null;
        pcRef.current?.close();
        pcRef.current = null;
      };
    } catch (err) {
      console.error("[host] getDisplayMedia failed:", err);
      setError(err instanceof Error ? err.name + ": " + err.message : "Failed to start screen share");
    }
  }, []);

  const handleWSMessage = useCallback(
    (msg: WSMessage) => {
      switch (msg.type) {
        case "guest-list":
          setGuests(msg.payload.guests as Guest[]);
          break;

        case "guest-joined":
          break;

        case "host-answer": {
          console.log("[host] answer received from SFU");
          const sdp = JSON.parse(msg.payload.sdp as string) as RTCSessionDescriptionInit;
          console.log("[host] answer SDP:", (sdp.sdp ?? "").substring(0, 200));
          pcRef.current?.setRemoteDescription(new RTCSessionDescription(sdp));
          break;
        }

        case "host-ice-candidate": {
          const candidate = JSON.parse(msg.payload.candidate as string) as RTCIceCandidateInit;
          console.log("[host] ICE candidate from SFU:", candidate.candidate?.substring(0, 80));
          pcRef.current?.addIceCandidate(new RTCIceCandidate(candidate));
          break;
        }

        case "offer": {
          console.log("[guest] offer received from SFU");
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
          console.log("[guest] offer SDP:", (sdp.sdp ?? "").substring(0, 200));
          pc.setRemoteDescription(new RTCSessionDescription(sdp))
            .then(async () => {
              console.log("[guest] setRemoteDescription OK");
              const answer = await pc.createAnswer();
              console.log("[guest] answer SDP:", (answer.sdp ?? "").substring(0, 200));
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

        case "ice-candidate": {
          const candidate = JSON.parse(msg.payload.candidate as string) as RTCIceCandidateInit;
          console.log("[guest] ICE candidate from SFU:", candidate.candidate?.substring(0, 80));
          pcRef.current?.addIceCandidate(new RTCIceCandidate(candidate));
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
    [role]
  );

  const kick = useCallback((guestId: string) => {
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