import { useParams, useNavigate } from "react-router";
import { useWebRTC } from "../hooks/useWebRTC";
import { useState, useEffect } from "react";

export function Guest() {
  const { sessionId } = useParams<{ sessionId: string }>();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [joined, setJoined] = useState(false);

  const { connected, remoteStream, error } = useWebRTC({
    sessionId: sessionId!,
    role: "guest",
    name: joined ? name : undefined,
  });

  useEffect(() => {
    if (error) {
      const t = setTimeout(() => navigate("/"), 3000);
      return () => clearTimeout(t);
    }
  }, [error, navigate]);

  if (!joined) {
    return (
      <div style={{ maxWidth: 400, margin: "100px auto", textAlign: "center" }}>
        <h1>Instant Share</h1>
        <p>Você foi convidado para assistir</p>
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

  return (
    <div style={{ maxWidth: 800, margin: "0 auto", padding: 20 }}>
      <h1>Instant Share</h1>
      <p>Assistindo como: {name}</p>

      {error ? (
        <div style={{ padding: 40, textAlign: "center", color: "red" }}>{error}</div>
      ) : (
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
              ref={(el) => {
                if (el) el.srcObject = remoteStream;
              }}
              autoPlay
              style={{ width: "100%", height: "100%", borderRadius: 8 }}
            />
          ) : (
            <p style={{ color: "#888" }}>Aguardando transmissão...</p>
          )}
        </div>
      )}

      <div style={{ marginTop: 20, color: connected ? "green" : "red" }}>
        {connected ? "Conectado" : "Desconectado"}
      </div>
    </div>
  );
}