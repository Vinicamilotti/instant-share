package config

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

var (
	SFUAdvertiseIP string

	iceServers   []webrtc.ICEServer
	iceServersMu sync.RWMutex
)

func Init() {
	SFUAdvertiseIP = os.Getenv("SFU_ADVERTISE_IP")

	if os.Getenv("ENABLE_TURN") != "true" {
		return
	}

	apiKey := os.Getenv("METERED_API_KEY")
	if apiKey == "" {
		log.Println("[config] ENABLE_TURN=true but METERED_API_KEY not set — TURN disabled")
		return
	}

	refreshICEServers(apiKey)

	go func() {
		for {
			time.Sleep(1 * time.Hour)
			refreshICEServers(apiKey)
		}
	}()
}

func refreshICEServers(apiKey string) {
	url := fmt.Sprintf("https://instant-screen-share.metered.live/api/v1/turn/credentials?apiKey=%s", apiKey)

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[config] TURN fetch error: %v", err)
		return
	}
	defer resp.Body.Close()

	var raw []struct {
		URLs       string `json:"urls"`
		Username   string `json:"username"`
		Credential string `json:"credential"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		log.Printf("[config] TURN decode error: %v", err)
		return
	}

	var servers []webrtc.ICEServer
	for _, r := range raw {
		servers = append(servers, webrtc.ICEServer{
			URLs:       []string{r.URLs},
			Username:   r.Username,
			Credential: r.Credential,
		})
	}

	iceServersMu.Lock()
	iceServers = servers
	iceServersMu.Unlock()

	log.Printf("[config] TURN credentials refreshed (%d servers)", len(servers))
}

func ICEServers() []webrtc.ICEServer {
	iceServersMu.RLock()
	defer iceServersMu.RUnlock()
	return iceServers
}

func ICEServerConfigs() []map[string]interface{} {
	servers := ICEServers()
	result := make([]map[string]interface{}, 0, len(servers))
	for _, s := range servers {
		result = append(result, map[string]interface{}{
			"urls":       s.URLs,
			"username":   s.Username,
			"credential": s.Credential,
		})
	}
	return result
}

func HasTURN() bool {
	iceServersMu.RLock()
	defer iceServersMu.RUnlock()
	return len(iceServers) > 0
}