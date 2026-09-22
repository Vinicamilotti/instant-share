package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"instant-share/id"
	"instant-share/model"
	"instant-share/sfu"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var (
	hostConns  sync.Map
	guestConns sync.Map
)

type safeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *safeConn) writeJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

func (c *safeConn) close() error {
	return c.conn.Close()
}

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
	GuestID string `json:"guest_id,omitempty"`
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
	sess := model.GetSession(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	sc := &safeConn{conn: conn}

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
			hostConns.Store(sessionID, sc)
			log.Printf("host joined session %s", sessionID)

			ensureRelay(sessionID, sess)

		case "guest-join":
			role = "guest"
			var p guestPayload
			json.Unmarshal(msg.Payload, &p)
			guestID, _ = id.New()

			guestConns.Store(key(sessionID, guestID), sc)
			model.AddGuest(sessionID, &model.Guest{ID: guestID, Name: p.Name})

			log.Printf("guest %s joined session %s", p.Name, sessionID)

			sendGuestList(sessionID)
			notifyHostGuestJoined(sessionID, guestID, p.Name)

			go tryCreateGuestOffer(sessionID, guestID)

		case "host-offer":
			var p offerPayload
			json.Unmarshal(msg.Payload, &p)
			log.Printf("[%s] host-offer received", sessionID)
			handleHostOffer(sessionID, p.SDP)

		case "host-ice-candidate":
			var p icePayload
			json.Unmarshal(msg.Payload, &p)
			relay := sess.Relay
			if relay != nil {
				relay.AddHostICECandidate(p.Candidate)
			}

		case "offer":
			var p offerPayload
			json.Unmarshal(msg.Payload, &p)
			log.Printf("[%s] offer → guest %s (legacy, ignored)", sessionID, p.GuestID)

		case "answer":
			var p answerPayload
			json.Unmarshal(msg.Payload, &p)
			p.GuestID = findGuestID(sessionID, conn)
			log.Printf("[%s] answer ← guest %s → sfu", sessionID, p.GuestID)
			relay := sess.Relay
			if relay != nil {
				relay.ProcessGuestAnswer(p.GuestID, p.SDP)
			}

		case "ice-candidate":
			var p icePayload
			json.Unmarshal(msg.Payload, &p)
			if isHost(sessionID, conn) {
				log.Printf("[%s] ICE host → guest %s (legacy, using host-ice-candidate)", sessionID, p.GuestID)
				relay := sess.Relay
				if relay != nil {
					relay.AddHostICECandidate(p.Candidate)
				}
			} else {
				p.GuestID = findGuestID(sessionID, conn)
				log.Printf("[%s] ICE guest %s → sfu", sessionID, p.GuestID)
				relay := sess.Relay
				if relay != nil {
					relay.AddGuestICECandidate(p.GuestID, p.Candidate)
				}
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

func handleHostOffer(sessionID, sdp string) {
	sess := model.GetSession(sessionID)
	if sess == nil || sess.Relay == nil {
		return
	}

	answerSDP, err := sess.Relay.ProcessHostOffer(sdp)
	if err != nil {
		log.Printf("[%s] host-offer processing error: %v", sessionID, err)
		return
	}

	payload, _ := json.Marshal(map[string]string{"sdp": answerSDP})
	sendToHost(sessionID, wsMessage{Type: "host-answer", Payload: payload})
	log.Printf("[%s] host-answer sent", sessionID)
}

func tryCreateGuestOffer(sessionID, guestID string) {
	sess := model.GetSession(sessionID)
	if sess == nil || sess.Relay == nil {
		return
	}

	if !sess.Relay.HasTrack() {
		log.Printf("[%s] guest %s waiting for host track...", sessionID, guestID)
		return
	}

	offerSDP, err := sess.Relay.CreateGuestSession(guestID)
	if err != nil {
		log.Printf("[%s] create guest session %s error: %v", sessionID, guestID, err)
		return
	}

	payload, _ := json.Marshal(map[string]string{"sdp": offerSDP})
	sendToGuest(sessionID, guestID, wsMessage{Type: "offer", Payload: payload})
	log.Printf("[%s] offer sent to guest %s", sessionID, guestID)
}

func ensureRelay(sessionID string, sess *model.Session) {
	if sess.Relay != nil {
		return
	}

	relay := sfu.NewRelay()

	relay.OnHostICECandidate = func(candidate string) {
		payload, _ := json.Marshal(map[string]string{"candidate": candidate})
		sendToHost(sessionID, wsMessage{Type: "host-ice-candidate", Payload: payload})
	}

	relay.OnTrackReady = func() {
		log.Printf("[%s] track ready, fanning out to guests", sessionID)
		guests := model.GetGuests(sessionID)
		for _, g := range guests {
			go tryCreateGuestOffer(sessionID, g.ID)
		}
	}

	relay.OnGuestICECandidate = func(guestID, candidate string) {
		payload, _ := json.Marshal(map[string]string{"candidate": candidate})
		sendToGuest(sessionID, guestID, wsMessage{Type: "ice-candidate", Payload: payload})
	}

	sess.Relay = relay
	log.Printf("[%s] SFU relay created", sessionID)
}

func key(sessionID, guestID string) string {
	return sessionID + ":" + guestID
}

func isHost(sessionID string, conn *websocket.Conn) bool {
	if hc, ok := hostConns.Load(sessionID); ok {
		return hc.(*safeConn).conn == conn
	}
	return false
}

func findGuestID(sessionID string, conn *websocket.Conn) string {
	prefix := sessionID + ":"
	var found string
	guestConns.Range(func(k, v any) bool {
		if v.(*safeConn).conn == conn {
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
	if sc, ok := hostConns.Load(sessionID); ok {
		sc.(*safeConn).writeJSON(msg)
	}
}

func sendToGuest(sessionID, guestID string, msg wsMessage) {
	if sc, ok := guestConns.Load(key(sessionID, guestID)); ok {
		sc.(*safeConn).writeJSON(msg)
	}
}

func broadcastToGuests(sessionID string, msg wsMessage) {
	prefix := sessionID + ":"
	guestConns.Range(func(k, v any) bool {
		if len(k.(string)) > len(prefix) && k.(string)[:len(prefix)] == prefix {
			v.(*safeConn).writeJSON(msg)
		}
		return true
	})
}

func handleKick(sessionID, guestID string) {
	k := key(sessionID, guestID)
	if sc, ok := guestConns.Load(k); ok {
		sc.(*safeConn).writeJSON(wsMessage{Type: "kicked"})
		sc.(*safeConn).conn.Close()
	}
	guestConns.Delete(k)
	model.RemoveGuest(sessionID, guestID)
	sendGuestList(sessionID)
}