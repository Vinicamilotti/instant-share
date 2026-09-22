package model

import (
	"sync"
	"time"

	"instant-share/sfu"

	"github.com/pion/webrtc/v4"
)

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
	Relay   *sfu.Relay
}

var (
	sessions = make(map[string]*Session)
	mu       sync.RWMutex
)

func CreateSession(id string) *Session {
	mu.Lock()
	defer mu.Unlock()
	s := &Session{
		ID:      id,
		Guests:  make(map[string]*Guest),
		Created: time.Now(),
	}
	sessions[id] = s
	return s
}

func GetSession(id string) *Session {
	mu.RLock()
	defer mu.RUnlock()
	return sessions[id]
}

func DeleteSession(id string) {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := sessions[id]; ok {
		if s.Relay != nil {
			s.Relay.Close()
		}
		if s.Host != nil {
			s.Host.Close()
		}
		for _, g := range s.Guests {
			if g.Conn != nil {
				g.Conn.Close()
			}
		}
		delete(sessions, id)
	}
}

func AddGuest(sessionID string, g *Guest) {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := sessions[sessionID]; ok {
		s.Guests[g.ID] = g
	}
}

func RemoveGuest(sessionID, guestID string) {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := sessions[sessionID]; ok {
		if g, exists := s.Guests[guestID]; exists {
			if g.Conn != nil {
				g.Conn.Close()
			}
			delete(s.Guests, guestID)
		}
		if s.Relay != nil {
			s.Relay.RemoveGuest(guestID)
		}
	}
}

func GetGuests(sessionID string) []Guest {
	mu.RLock()
	defer mu.RUnlock()
	s := sessions[sessionID]
	if s == nil {
		return nil
	}
	guests := make([]Guest, 0, len(s.Guests))
	for _, g := range s.Guests {
		guests = append(guests, Guest{ID: g.ID, Name: g.Name})
	}
	return guests
}