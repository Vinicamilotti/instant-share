package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"instant-share/id"
	"instant-share/model"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var (
	hostConns  sync.Map
	guestConns sync.Map
)

type wsMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type guestPayload struct {
	Name string `json:"name"`
}

type kickPayload struct {
	GuestID string `json:"guest_id"`
}

type offerPayload struct {
	SDP     string `json:"sdp"`
	GuestID string `json:"guest_id"`
}

type answerPayload struct {
	SDP     string `json:"sdp"`
	GuestID string `json:"guest_id,omitempty"`
}

type icePayload struct {
	Candidate string `json:"candidate"`
	GuestID   string `json:"guest_id,omitempty"`
}

func WebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("session_id")
	if model.GetSession(sessionID) == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	var role string
	var guestID string

	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		switch msg.Type {
		case "host-join":
			role = "host"
			hostConns.Store(sessionID, conn)
			log.Printf("host joined session %s", sessionID)

		case "guest-join":
			role = "guest"
			var p guestPayload
			json.Unmarshal(msg.Payload, &p)
			guestID, _ = id.New()

			guestConns.Store(key(sessionID, guestID), conn)
			model.AddGuest(sessionID, &model.Guest{ID: guestID, Name: p.Name})

			log.Printf("guest %s joined session %s", p.Name, sessionID)

			sendGuestList(sessionID)
			notifyHostGuestJoined(sessionID, guestID, p.Name)

		case "offer":
			var p offerPayload
			json.Unmarshal(msg.Payload, &p)
			log.Printf("[%s] offer → guest %s", sessionID, p.GuestID)
			sendToGuest(sessionID, p.GuestID, msg)

		case "answer":
			var p answerPayload
			json.Unmarshal(msg.Payload, &p)
			p.GuestID = findGuestID(sessionID, conn)
			data, _ := json.Marshal(p)
			log.Printf("[%s] answer ← guest %s → host", sessionID, p.GuestID)
			sendToHost(sessionID, wsMessage{Type: "answer", Payload: data})

		case "ice-candidate":
			var p icePayload
			json.Unmarshal(msg.Payload, &p)
			if isHost(sessionID, conn) {
				log.Printf("[%s] ICE host → guest %s", sessionID, p.GuestID)
				sendToGuest(sessionID, p.GuestID, msg)
			} else {
				p.GuestID = findGuestID(sessionID, conn)
				data, _ := json.Marshal(p)
				log.Printf("[%s] ICE guest %s → host", sessionID, p.GuestID)
				sendToHost(sessionID, wsMessage{Type: "ice-candidate", Payload: data})
			}

		case "kick":
			var p kickPayload
			json.Unmarshal(msg.Payload, &p)
			handleKick(sessionID, p.GuestID)
		}
	}

	if role == "host" {
		hostConns.Delete(sessionID)
		broadcastToGuests(sessionID, wsMessage{Type: "session-ended"})
		model.DeleteSession(sessionID)
		log.Printf("host disconnected from session %s", sessionID)
	} else if role == "guest" && guestID != "" {
		guestConns.Delete(key(sessionID, guestID))
		model.RemoveGuest(sessionID, guestID)
		sendGuestList(sessionID)
		log.Printf("guest %s left session %s", guestID, sessionID)
	}
}

func key(sessionID, guestID string) string {
	return sessionID + ":" + guestID
}

func isHost(sessionID string, conn *websocket.Conn) bool {
	if hc, ok := hostConns.Load(sessionID); ok && hc == conn {
		return true
	}
	return false
}

func findGuestID(sessionID string, conn *websocket.Conn) string {
	prefix := sessionID + ":"
	var found string
	guestConns.Range(func(k, v any) bool {
		if v == conn {
			s := k.(string)
			if len(s) > len(prefix) {
				found = s[len(prefix):]
			}
			return false
		}
		return true
	})
	if found == "" {
		log.Printf("[%s] findGuestID: NOT FOUND for conn %p", sessionID, conn)
	}
	return found
}

func sendGuestList(sessionID string) {
	guests := model.GetGuests(sessionID)
	payload := map[string]any{"guests": guests}
	data, _ := json.Marshal(payload)
	sendToHost(sessionID, wsMessage{Type: "guest-list", Payload: data})
}

func notifyHostGuestJoined(sessionID, guestID, name string) {
	payload := map[string]any{"id": guestID, "name": name}
	data, _ := json.Marshal(payload)
	sendToHost(sessionID, wsMessage{Type: "guest-joined", Payload: data})
}

func sendToHost(sessionID string, msg wsMessage) {
	if conn, ok := hostConns.Load(sessionID); ok {
		conn.(*websocket.Conn).WriteJSON(msg)
	}
}

func sendToGuest(sessionID, guestID string, msg wsMessage) {
	if conn, ok := guestConns.Load(key(sessionID, guestID)); ok {
		conn.(*websocket.Conn).WriteJSON(msg)
	}
}

func broadcastToGuests(sessionID string, msg wsMessage) {
	prefix := sessionID + ":"
	guestConns.Range(func(k, v any) bool {
		if len(k.(string)) > len(prefix) && k.(string)[:len(prefix)] == prefix {
			v.(*websocket.Conn).WriteJSON(msg)
		}
		return true
	})
}

func handleKick(sessionID, guestID string) {
	k := key(sessionID, guestID)
	if conn, ok := guestConns.Load(k); ok {
		conn.(*websocket.Conn).WriteJSON(wsMessage{Type: "kicked"})
		conn.(*websocket.Conn).Close()
	}
	guestConns.Delete(k)
	model.RemoveGuest(sessionID, guestID)
	sendGuestList(sessionID)
}