# Instant Share — Documentação Técnica

Aplicação web de screen sharing. Um host inicia o compartilhamento de tela, recebe um link curto e convidados entram para assistir em tempo real. O host vê quem está assistindo e pode remover (kick) qualquer guest.

---

## Arquitetura

```
┌──────────────────────────────────────────────────────────────────┐
│  instant-share (binário único)                                    │
│                                                                   │
│  ┌──────────────────────┐    ┌──────────────────────────────────┐ │
│  │  React SPA (embed)   │    │  Backend Go                      │ │
│  │                      │◄──►│  • API REST                      │ │
│  │  • Vite build → dist │    │  • WebSocket signaling           │ │
│  │  • //go:embed        │    │  • WebRTC relay (Pion)           │ │
│  └──────────────────────┘    └──────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
         ▲                              ▲
         │                              │
    WebRTC (mídia)              WebSocket (signaling)
         │                              │
    ┌────┴────┐                  ┌──────┴──────┐
    │  Host   │                  │   Guests    │
    └─────────┘                  └─────────────┘
```

- **React SPA** compilado com Vite, build em `dist/`, embutido no binário Go via `//go:embed`
- **Backend Go** serve os arquivos estáticos do React, expõe API REST e WebSocket, faz relay WebRTC
- **WebSocket** usado para signaling (SDP, ICE) e eventos de sessão (join, leave, kick)
- **WebRTC** transporte de mídia (áudio/vídeo da tela) via backend como relay

---

## Estrutura do Projeto

```
instant-share/
├── main.go
├── go.mod
├── handler/
│   ├── session.go         # POST /api/host-session
│   └── ws.go              # WebSocket /ws/:session_id
├── model/
│   └── session.go         # struct Session, Guest
├── id/
│   └── id.go              # gerador de ID de sessão
├── sfu/
│   └── relay.go           # relay WebRTC (Pion)
└── web/                   # React SPA (Vite)
    ├── index.html
    ├── vite.config.ts
    ├── package.json
    ├── src/
    │   ├── main.tsx
    │   ├── App.tsx
    │   ├── pages/
    │   │   ├── Home.tsx           # landing: nome + iniciar
    │   │   ├── Host.tsx           # preview + lista de guests + kick
    │   │   └── Guest.tsx          # player de tela compartilhada
    │   ├── hooks/
    │   │   └── useWebRTC.ts       # hook WebRTC (offer/answer/ICE)
    │   └── lib/
    │       ├── api.ts             # chamadas REST
    │       ├── ws.ts              # cliente WebSocket
    │       └── types.ts           # tipos compartilhados
    └── dist/               # output do build (gerado, .gitignore)
```

---

## Backend (Go)

### Modelo de Dados

```go
type Guest struct {
    ID   string `json:"id"`
    Name string `json:"name"`
    Conn *webrtc.PeerConnection
}

type Session struct {
    ID      string
    Host    *webrtc.PeerConnection
    Guests  map[string]*Guest
    Created time.Time
}
```

Armazenamento em memória: `sync.Map` ou `map[string]*Session` com `sync.RWMutex`.

### API REST

#### `POST /api/host-session`

Cria uma sessão de screen sharing.

**Request:**
```json
{ "name": "João" }
```

**Response:**
```json
{
  "session_id": "xg7k2m9p4a",
  "link": "https://instant-share.xyz/xg7k2m9p4a"
}
```

1. Gera ID de sessão aleatório (10 chars, `[a-z0-9]`)
2. Cria `Session` em memória
3. Retorna o link mágico

### WebSocket

#### `WS /ws/:session_id`

Cada mensagem JSON tem o formato:

```json
{ "type": "<tipo>", "payload": { ... } }
```

**Cliente → Servidor:**

| Type | Payload | Descrição |
|------|---------|-----------|
| `host-join` | `{}` | Host se conecta à sessão |
| `guest-join` | `{ "name": "..." }` | Guest informa nome |
| `offer` | `{ "sdp": "..." }` | SDP offer do host |
| `answer` | `{ "sdp": "..." }` | SDP answer do guest |
| `ice-candidate` | `{ "candidate": "..." }` | ICE candidate |
| `kick` | `{ "guest_id": "..." }` | Host remove guest |

**Servidor → Cliente:**

| Type | Payload | Descrição |
|------|---------|-----------|
| `guest-list` | `{ "guests": [...] }` | Lista de guests (enviada ao host) |
| `offer` | `{ "sdp": "..." }` | Offer encaminhada ao guest |
| `answer` | `{ "sdp": "..." }` | Answer encaminhada ao host |
| `ice-candidate` | `{ "candidate": "..." }` | ICE candidate encaminhado |
| `kicked` | `{}` | Guest foi removido |
| `session-ended` | `{}` | Sessão encerrada pelo host |

### Servir o Frontend

O backend serve os arquivos estáticos do React embutidos:

```go
//go:embed web/dist/*
var staticFiles embed.FS

// Rotas:
// /api/*          → handlers da API
// /ws/*           → WebSocket
// /*              → arquivos estáticos do React (SPA fallback → index.html)
```

Rota catch-all `/*` retorna `index.html` para suportar client-side routing do React Router.

---

## Frontend (React + Vite)

### Stack

| Ferramenta | Escolha |
|-----------|---------|
| Bundler | Vite |
| Routing | React Router v7 |
| Estilo | CSS Modules ou Tailwind |
| Estado | `useState` + `useReducer` (sem lib externa na v1) |
| WebSocket | `native WebSocket` (navegador) |
| WebRTC | `RTCPeerConnection` nativa do navegador |

### Rotas

| Path | Componente | Descrição |
|------|-----------|-----------|
| `/` | `Home` | Landing page: input de nome + botão "Iniciar" |
| `/host/:sessionId` | `Host` | Preview da tela + lista de guests + botão kick |
| `/guest/:sessionId` | `Guest` | Player da tela compartilhada |

### Telas

#### `Home` (`/`)

- Campo de texto: "Seu nome"
- Botão "Iniciar"
- Ao clicar: chama `POST /api/host-session` → redireciona para `/host/:sessionId`
- Se acessar `/guest/:sessionId` diretamente (via link), mostra input de nome e redireciona para `/guest/:sessionId`

#### `Host` (`/host/:sessionId`)

- Preview da própria tela (`<video>` com stream local)
- Link de compartilhamento com botão de copiar
- Lista de guests assistindo (atualizada em tempo real via WS)
- Botão "kick" em cada guest
- Ao fechar/quando desconecta: backend encerra a sessão automaticamente

Estado gerenciado:
- `localStream: MediaStream | null`
- `guests: Guest[]`
- `sessionLink: string`

#### `Guest` (`/guest/:sessionId`)

- Player de vídeo (`<video>` com stream remoto)
- Nome exibido (recebido do convite)
- Indicador "Assistindo"
- Ao receber `session-ended` ou `kicked`: mostra mensagem e redireciona

Estado gerenciado:
- `remoteStream: MediaStream | null`
- `connected: boolean`

### Hook `useWebRTC`

Hook principal que encapsula toda a lógica WebRTC + WebSocket:

```ts
function useWebRTC(sessionId: string, role: "host" | "guest", name?: string) {
  // retorna:
  return {
    localStream,   // MediaStream (host apenas)
    remoteStream,  // MediaStream (guest apenas)
    guests,        // Guest[] (host apenas)
    sessionLink,   // string (host apenas)
    connected,     // boolean
    kick,          // (guestId: string) => void
    error,         // string | null
  };
}
```

Internamente:
1. Conecta WebSocket em `/ws/:sessionId`
2. Envia `host-join` ou `guest-join`
3. Se host: obtém `getDisplayMedia()`, cria offer, envia SDP + ICE pelo WS
4. Se guest: recebe offer, cria answer, envia de volta
5. Escuta mensagens WS: `guest-list`, `offer`, `answer`, `ice-candidate`, `kicked`, `session-ended`
6. Cleanup: fecha PeerConnection e WebSocket no unmount

### Tipos compartilhados (`types.ts`)

```ts
interface Guest {
  id: string;
  name: string;
}

interface WSMessage {
  type: string;
  payload: Record<string, unknown>;
}
```

---

## Fluxo Principal

### Host inicia o share

1. Acessa `/` → insere nome → clica "Iniciar"
2. `POST /api/host-session` → recebe `session_id` + link
3. Redireciona para `/host/:sessionId`
4. `useWebRTC` conecta WS, envia `host-join`
5. `getDisplayMedia()` → cria `RTCPeerConnection` → gera offer
6. Offer + ICE candidates fluem via WS → backend → guests
7. Preview local aparece no `<video>`, guests começam a aparecer na lista

### Guest entra na sessão

1. Acessa `/<sessionId>` → insere nome
2. `useWebRTC` conecta WS, envia `guest-join`
3. Backend adiciona guest à sessão, notifica host com `guest-list`
4. Guest recebe offer → cria answer → envia de volta
5. `remoteStream` é setado no `<video>`

### Host gerencia guests

1. Host recebe `guest-list` via WS sempre que há mudança
2. Clica "kick" em um guest → envia `{ type: "kick", payload: { guest_id } }`
3. Backend fecha PeerConnection do guest, envia `kicked`

### Encerramento

1. Host fecha a página → WS desconecta
2. Backend detecta close, envia `session-ended` para todos os guests
3. Remove `Session` do mapa em memória

---

## Decisões Técnicas

| Decisão | Escolha | Motivo |
|---------|---------|--------|
| ID de sessão | 10 chars `[a-z0-9]` via `crypto/rand` | Curto, fácil de compartilhar |
| WebRTC SFU | [Pion](https://github.com/pion/webrtc) | Biblioteca nativa Go, sem dependência externa |
| WebSocket | [gorilla/websocket](https://github.com/gorilla/websocket) | Madura, simples |
| Binário único | `//go:embed` + Vite build | Deploy trivial, sem servir estáticos separados |
| Frontend | React + Vite + React Router | SPA leve, componentização para estado WebRTC |
| Roteamento | React Router (client-side) | Essencial para as rotas `/`, `/host/:id`, `/guest/:id` |
| Estado | `useState`/`useReducer` | Número pequeno de estados, sem necessidade de lib externa |

---

## Requisitos Não-Funcionais

- Latência alvo: < 500ms entre host e guest
- Sessão expira automaticamente se host desconectar (sem timeout de inatividade na v1)
- Sem autenticação na v1 — qualquer pessoa com o link pode assistir
- Sem gravação na v1