import { useState, useRef, useEffect } from "react";
import { useParams, useNavigate } from "react-router";
import { useWebRTC } from "../hooks/useWebRTC";

export function Guest() {
  const { sessionId } = useParams<{ sessionId: string }>();
  const [name, setName] = useState("");
  const [joined, setJoined] = useState(false);

  if (joined) {
    return <GuestStream sessionId={sessionId!} name={name} />;
  }

  return (
    <div style={{ maxWidth: 400, margin: "100px auto", textAlign: "center" }}>
      <h1>Instant Share</h1>
      <p>Voce foi convidado para assistir</p>
      <input
        type="text"
        placeholder="Seu nome"
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && name.trim() && setJoined(true)}
        style={{ width: "100%", padding: 12, fontSize: 16, marginBottom: 16, boxSizing: "border-box" }}
      />
      <button
        onClick={() => setJoined(true)}
        disabled={!name.trim()}
        style={{ width: "100%", padding: 12, fontSize: 16 }}
      >
        Entrar
      </button>
    </div>
  );
}

function GuestStream({ sessionId, name }: { sessionId: string; name: string }) {
  const navigate = useNavigate();
  const videoRef = useRef<HTMLVideoElement>(null);
  const { connected, remoteStream, error } = useWebRTC({
    sessionId,
    role: "guest",
    name,
  });

  useEffect(() => {
    if (videoRef.current && remoteStream) {
      videoRef.current.srcObject = remoteStream;
    }
  }, [remoteStream]);

  if (error) {
    return (
      <div style={{ maxWidth: 400, margin: "100px auto", textAlign: "center" }}>
        <p style={{ color: "red" }}>{error}</p>
        <button onClick={() => navigate("/")} style={{ padding: "8px 16px", marginTop: 16 }}>
          Voltar ao inicio
        </button>
      </div>
    );
  }

  return (
    <div style={{ maxWidth: 800, margin: "0 auto", padding: 20 }}>
      <h1>Instant Share</h1>
      <p>Assistindo como: {name}</p>

      <div
        style={{
          width: "100%",
          height: 400,
          background: "#1a1a1a",
          borderRadius: 8,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        {remoteStream ? (
          <video
            ref={videoRef}
            autoPlay
            style={{ width: "100%", height: "100%", borderRadius: 8 }}
          />
        ) : (
          <p style={{ color: "#888" }}>Aguardando transmissao...</p>
        )}
      </div>

      <div style={{ marginTop: 20, color: connected ? "green" : "red" }}>
        {connected ? "Conectado" : "Desconectado"}
      </div>
    </div>
  );
}