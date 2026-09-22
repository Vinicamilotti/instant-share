package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"instant-share/id"
	"instant-share/model"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type guestPayload struct {
	Name string `json:"name"`
}

type sdpPayload struct {
	SDP string `json:"sdp"`
}

type icePayload struct {
	Candidate string `json:"candidate"`
}

type kickPayload struct {
	GuestID string `json:"guest_id"`
}

func WebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	session := model.GetSession(sessionID)
	if session == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		switch msg.Type {
		case "host-join":
			handleHostJoin(sessionID, session, conn)
		case "guest-join":
			handleGuestJoin(sessionID, conn, msg.Payload)
		case "offer":
			broadcastToGuests(sessionID, msg, conn)
		case "answer":
			sendToHost(session, msg)
		case "ice-candidate":
			relayICE(sessionID, session, msg, conn)
		case "kick":
			handleKick(sessionID, session, msg.Payload)
		}
	}
}

func handleHostJoin(sessionID string, session *model.Session, conn *websocket.Conn) {
	log.Printf("host joined session %s", sessionID)

	conn.SetCloseHandler(func(code int, text string) error {
		log.Printf("host disconnected from session %s", sessionID)
		broadcastToGuests(sessionID, wsMessage{Type: "session-ended"}, nil)
		model.DeleteSession(sessionID)
		return nil
	})
}

func handleGuestJoin(sessionID string, conn *websocket.Conn, payload json.RawMessage) {
	var p guestPayload
	json.Unmarshal(payload, &p)

	guestID, _ := id.New()

	guest := &model.Guest{ID: guestID, Name: p.Name, Conn: nil}
	model.AddGuest(sessionID, guest)

	log.Printf("guest %s joined session %s", p.Name, sessionID)

	conn.SetCloseHandler(func(code int, text string) error {
		model.RemoveGuest(sessionID, guestID)
		sendGuestList(sessionID)
		return nil
	})

	sendGuestList(sessionID)
}

func sendGuestList(sessionID string) {
	guests := model.GetGuests(sessionID)
	data, _ := json.Marshal(guests)
	msg := wsMessage{Type: "guest-list", Payload: data}
	broadcastToHost(sessionID, msg)
}

func broadcastToGuests(sessionID string, msg wsMessage, sender *websocket.Conn) {
	// NOTE: in v1, WS is per-connection. For full broadcast, we'd need to track WS conns per guest.
	// For now, the WebRTC relay handles media distribution.
}

func sendToHost(session *model.Session, msg wsMessage) {
	// TODO: track host WS conn to send answer/ICE
}

func relayICE(sessionID string, session *model.Session, msg wsMessage, sender *websocket.Conn) {
	// TODO: relay ICE between host and guests
}

func broadcastToHost(sessionID string, msg wsMessage) {
	// TODO: send to host WS conn
}

func handleKick(sessionID string, session *model.Session, payload json.RawMessage) {
	var p kickPayload
	json.Unmarshal(payload, &p)

	model.RemoveGuest(sessionID, p.GuestID)
	sendGuestList(sessionID)
}