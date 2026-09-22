import { useParams } from "react-router";
import { useWebRTC } from "../hooks/useWebRTC";
import type { QualityPreset } from "../lib/types";
import { useEffect, useRef, useState } from "react";

export function Host() {
  const { sessionId } = useParams<{ sessionId: string }>();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [quality, setQuality] = useState<QualityPreset>('high');
  const {
    connected,
    localStream,
    guests,
    error,
    startScreenShare,
    kick,
  } = useWebRTC({ sessionId: sessionId!, role: "host", quality });

  const [link, setLink] = useState("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    setLink(`${location.origin}/${sessionId}`);
  }, [sessionId]);

  useEffect(() => {
    if (videoRef.current && localStream) {
      videoRef.current.srcObject = localStream;
    }
  }, [localStream]);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(link);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  if (error) {
    return <div style={{ padding: 40, textAlign: "center" }}>{error}</div>;
  }

  return (
    <div style={{ maxWidth: 800, margin: "0 auto", padding: 20 }}>
      <h1>Instant Share</h1>

      <div style={{ marginBottom: 20 }}>
        <input
          type="text"
          value={link}
          readOnly
          style={{ width: "100%", padding: 10, fontSize: 16, boxSizing: "border-box" }}
        />
        <button onClick={handleCopy} style={{ marginTop: 8, padding: "8px 16px" }}>
          {copied ? "Copiado!" : "Copiar link"}
        </button>
      </div>

      <div
        style={{
          width: "100%",
          height: 300,
          background: "#1a1a1a",
          borderRadius: 8,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          marginBottom: 20,
        }}
      >
        {localStream ? (
          <video
            ref={videoRef}
            autoPlay
            muted
            style={{ width: "100%", height: "100%", borderRadius: 8 }}
          />
        ) : (
          <div>
            <div style={{ marginBottom: 12 }}>
              <label htmlFor="quality" style={{ marginRight: 8 }}>Qualidade:</label>
              <select
                id="quality"
                value={quality}
                onChange={e => setQuality(e.target.value as QualityPreset)}
                style={{ padding: "6px 12px", fontSize: 14 }}
              >
                <option value="low">Baixa (720p)</option>
                <option value="medium">Media (1080p)</option>
                <option value="high">Alta (1080p, bitrate alto)</option>
                <option value="ultra">Ultra (1440p)</option>
              </select>
            </div>
            <button onClick={startScreenShare} style={{ padding: "12px 24px", fontSize: 16 }}>
              Compartilhar tela
            </button>
          </div>
        )}
      </div>

      <h2>Assistindo ({guests.length})</h2>
      {guests.length === 0 ? (
        <p style={{ color: "#888" }}>Ninguém assistindo ainda...</p>
      ) : (
        <ul style={{ listStyle: "none", padding: 0 }}>
          {guests.map((g) => (
            <li
              key={g.id}
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                padding: "10px 0",
                borderBottom: "1px solid #333",
              }}
            >
              <span>{g.name}</span>
              <button onClick={() => kick(g.id)} style={{ padding: "4px 12px" }}>
                Kick
              </button>
            </li>
          ))}
        </ul>
      )}

      <div style={{ marginTop: 20, color: connected ? "green" : "red" }}>
        {connected ? "Conectado" : "Desconectado"}
      </div>
    </div>
  );
}