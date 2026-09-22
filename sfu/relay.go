package sfu

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"instant-share/config"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

type Relay struct {
	mu          sync.Mutex
	api         *webrtc.API
	hostPC      *webrtc.PeerConnection
	guestPCs    map[string]*webrtc.PeerConnection
	hasTrack    bool
	codecCap    *webrtc.RTPCodecCapability
	videoTrack  *webrtc.TrackLocalStaticRTP
	audioTrack  *webrtc.TrackLocalStaticRTP
	videoSSRC   uint32

	pendingHostCandidates  []webrtc.ICECandidateInit
	pendingGuestCandidates map[string][]webrtc.ICECandidateInit

	OnHostICECandidate  func(candidate string)
	OnTrackReady        func()
	OnGuestICECandidate func(guestID, candidate string)
}

func NewRelay() *Relay {
	m := &webrtc.MediaEngine{}

	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack"}, webrtc.RTPCodecTypeVideo)
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack", Parameter: "pli"}, webrtc.RTPCodecTypeVideo)
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "transport-cc"}, webrtc.RTPCodecTypeVideo)

	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000},
		PayloadType:        102,
	}, webrtc.RTPCodecTypeVideo)

	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo)

	m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio)

	se := webrtc.SettingEngine{}
	se.SetLite(true)
	se.SetICETimeouts(10*time.Second, 10*time.Second, 2*time.Second)
	se.SetInterfaceFilter(func(name string) bool {
		if name == "lo" || strings.HasPrefix(name, "lo") {
			return true
		}
		if strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "en") || strings.HasPrefix(name, "wl") {
			return true
		}
		return false
	})

	if ip := config.SFUAdvertiseIP; ip != "" {
		se.SetNAT1To1IPs([]string{ip}, webrtc.ICECandidateTypeHost)
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(m),
		webrtc.WithSettingEngine(se),
	)

	return &Relay{
		api:                   api,
		guestPCs:              make(map[string]*webrtc.PeerConnection),
		pendingGuestCandidates: make(map[string][]webrtc.ICECandidateInit),
	}
}

func (r *Relay) peerConfig() webrtc.Configuration {
	return webrtc.Configuration{
		ICEServers: config.ICEServers(),
	}
}

func (r *Relay) ProcessHostOffer(sdpStr string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var offer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(sdpStr), &offer); err != nil {
		return "", fmt.Errorf("unmarshal host offer: %w", err)
	}

	pc, err := r.api.NewPeerConnection(r.peerConfig())
	if err != nil {
		return "", fmt.Errorf("create host pc: %w", err)
	}
	r.hostPC = pc

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[sfu] received track: %s, codec: %s, ssrc: %d", track.Kind(), track.Codec().MimeType, track.SSRC())

		r.mu.Lock()
		if !r.hasTrack {
			r.hasTrack = true
			codec := track.Codec().RTPCodecCapability
			r.codecCap = &codec

			if track.Kind() == webrtc.RTPCodecTypeVideo {
				r.videoSSRC = uint32(track.SSRC())
			}

			localTrack, err := webrtc.NewTrackLocalStaticRTP(codec, track.ID(), track.StreamID())
			if err != nil {
				log.Printf("[sfu] create local track error: %v", err)
				r.mu.Unlock()
				return
			}
			if track.Kind() == webrtc.RTPCodecTypeAudio {
				r.audioTrack = localTrack
			} else {
				r.videoTrack = localTrack
			}

			r.mu.Unlock()

			go func() {
				buf := make([]byte, 1500)
				for {
					i, _, readErr := track.Read(buf)
					if readErr != nil {
						log.Printf("[sfu] track read ended: %v", readErr)
						return
					}
					if _, writeErr := localTrack.Write(buf[:i]); writeErr != nil {
						log.Printf("[sfu] local track write error: %v", writeErr)
					}
				}
			}()

			if r.OnTrackReady != nil {
				r.OnTrackReady()
			}
		} else {
			r.mu.Unlock()
		}
	})

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			log.Printf("[sfu] host ICE gathering complete")
			return
		}
		log.Printf("[sfu] host ICE candidate: %s:%d %s", c.Address, c.Port, c.Typ)
		candidateJSON, err := json.Marshal(c.ToJSON())
		if err != nil {
			return
		}
		if r.OnHostICECandidate != nil {
			r.OnHostICECandidate(string(candidateJSON))
		}
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[sfu] host connection state: %s", state)
	})

	if err := pc.SetRemoteDescription(offer); err != nil {
		return "", fmt.Errorf("set host remote desc: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("create answer: %w", err)
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("set host local desc: %w", err)
	}

	answerJSON, err := json.Marshal(answer)
	if err != nil {
		return "", fmt.Errorf("marshal answer: %w", err)
	}

	for _, c := range r.pendingHostCandidates {
		pc.AddICECandidate(c)
	}
	r.pendingHostCandidates = nil

	return string(answerJSON), nil
}

func (r *Relay) AddHostICECandidate(candidateStr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(candidateStr), &candidate); err != nil {
		return fmt.Errorf("unmarshal ice candidate: %w", err)
	}

	if r.hostPC == nil {
		r.pendingHostCandidates = append(r.pendingHostCandidates, candidate)
		return nil
	}

	return r.hostPC.AddICECandidate(candidate)
}

func (r *Relay) CreateGuestSession(guestID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.hasTrack || r.videoTrack == nil {
		return "", fmt.Errorf("host track not ready")
	}

	pc, err := r.api.NewPeerConnection(r.peerConfig())
	if err != nil {
		return "", fmt.Errorf("create guest pc: %w", err)
	}
	r.guestPCs[guestID] = pc

	var videoSender *webrtc.RTPSender
	if r.videoTrack != nil {
		sender, err := pc.AddTrack(r.videoTrack)
		if err != nil {
			return "", fmt.Errorf("add video track: %w", err)
		}
		videoSender = sender
	}
	if r.audioTrack != nil {
		if _, err := pc.AddTrack(r.audioTrack); err != nil {
			return "", fmt.Errorf("add audio track: %w", err)
		}
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			log.Printf("[sfu] guest %s ICE gathering complete", guestID)
			return
		}
		log.Printf("[sfu] guest %s ICE candidate: %s:%d %s", guestID, c.Address, c.Port, c.Typ)
		candidateJSON, err := json.Marshal(c.ToJSON())
		if err != nil {
			return
		}
		if r.OnGuestICECandidate != nil {
			r.OnGuestICECandidate(guestID, string(candidateJSON))
		}
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[sfu] guest %s connection state: %s", guestID, state)
		if state == webrtc.PeerConnectionStateConnected {
			r.mu.Lock()
			r.requestKeyframeLocked()
			r.mu.Unlock()
		}
	})

	if videoSender != nil {
		hostPC := r.hostPC
		guest := guestID
		go relayRTCP(guest, videoSender, hostPC)
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("create guest offer: %w", err)
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		return "", fmt.Errorf("set guest local desc: %w", err)
	}

	for _, c := range r.pendingGuestCandidates[guestID] {
		pc.AddICECandidate(c)
	}
	delete(r.pendingGuestCandidates, guestID)

	offerJSON, err := json.Marshal(offer)
	if err != nil {
		return "", fmt.Errorf("marshal guest offer: %w", err)
	}

	log.Printf("[sfu] created guest session %s", guestID)
	return string(offerJSON), nil
}

func (r *Relay) requestKeyframeLocked() {
	if r.hostPC == nil || r.videoSSRC == 0 {
		return
	}
	pli := &rtcp.PictureLossIndication{
		MediaSSRC: r.videoSSRC,
	}
	if err := r.hostPC.WriteRTCP([]rtcp.Packet{pli}); err != nil {
		log.Printf("[sfu] PLI send error: %v", err)
	} else {
		log.Printf("[sfu] PLI sent to host (ssrc=%d)", r.videoSSRC)
	}
}

func relayRTCP(guestID string, sender *webrtc.RTPSender, hostPC *webrtc.PeerConnection) {
	for {
		pkts, _, readErr := sender.ReadRTCP()
		if readErr != nil {
			log.Printf("[sfu] rtcp relay %s read ended: %v", guestID, readErr)
			return
		}
		if hostPC != nil {
			for _, p := range pkts {
				switch p.(type) {
				case *rtcp.PictureLossIndication:
					log.Printf("[sfu] rtcp relay %s → host: PLI", guestID)
				}
			}
			if err := hostPC.WriteRTCP(pkts); err != nil {
				log.Printf("[sfu] rtcp relay write error: %v", err)
			}
		}
	}
}

func (r *Relay) ProcessGuestAnswer(guestID, sdpStr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	pc := r.guestPCs[guestID]
	if pc == nil {
		return fmt.Errorf("guest %s not found", guestID)
	}

	var answer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(sdpStr), &answer); err != nil {
		return fmt.Errorf("unmarshal guest answer: %w", err)
	}

	return pc.SetRemoteDescription(answer)
}

func (r *Relay) AddGuestICECandidate(guestID, candidateStr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(candidateStr), &candidate); err != nil {
		return fmt.Errorf("unmarshal guest ice candidate: %w", err)
	}

	pc := r.guestPCs[guestID]
	if pc == nil {
		if r.pendingGuestCandidates[guestID] == nil {
			r.pendingGuestCandidates[guestID] = make([]webrtc.ICECandidateInit, 0)
		}
		r.pendingGuestCandidates[guestID] = append(r.pendingGuestCandidates[guestID], candidate)
		return nil
	}

	return pc.AddICECandidate(candidate)
}

func (r *Relay) HasTrack() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hasTrack
}

func (r *Relay) RemoveGuest(guestID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if pc, ok := r.guestPCs[guestID]; ok {
		pc.Close()
		delete(r.guestPCs, guestID)
	}
}

func (r *Relay) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hostPC != nil {
		r.hostPC.Close()
	}
	for _, pc := range r.guestPCs {
		pc.Close()
	}
	r.guestPCs = make(map[string]*webrtc.PeerConnection)
}