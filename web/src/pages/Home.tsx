import { useState } from "react";
import { useNavigate } from "react-router";
import { createSession } from "../lib/api";

export function Home() {
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleStart = async () => {
    if (!name.trim()) return;
    setLoading(true);
    try {
      const { session_id } = await createSession(name);
      navigate(`/host/${session_id}`);
    } catch {
      setLoading(false);
    }
  };

  return (
    <div style={{ maxWidth: 400, margin: "100px auto", textAlign: "center" }}>
      <h1>Instant Share</h1>
      <input
        type="text"
        placeholder="Seu nome"
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && handleStart()}
        style={{
          width: "100%",
          padding: 12,
          fontSize: 16,
          marginBottom: 16,
          boxSizing: "border-box",
        }}
      />
      <button
        onClick={handleStart}
        disabled={loading || !name.trim()}
        style={{ width: "100%", padding: 12, fontSize: 16 }}
      >
        {loading ? "Criando..." : "Iniciar"}
      </button>
    </div>
  );
}