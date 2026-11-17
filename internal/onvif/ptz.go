package onvif

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
)

const ptzLease = 1500 * time.Millisecond

type ptzCommand struct {
	Source string  `json:"src"`
	Action string  `json:"action"`
	Pan    float64 `json:"pan"`
	Tilt   float64 `json:"tilt"`
	Zoom   float64 `json:"zoom"`
	Preset string  `json:"preset,omitempty"`
}

// Each source has its own lock: a slow camera never blocks another camera.
// Clients survive button presses, so a warm command needs only one SOAP request.
type ptzSession struct {
	mu       sync.Mutex
	client   *onvif.Client
	token    string
	timer    *time.Timer
	deadline time.Time
	lastUsed time.Time
	users    int
	discard  bool
}

var ptzSessions = struct {
	sync.Mutex
	items     map[string]*ptzSession
	nextSweep time.Time
}{items: make(map[string]*ptzSession)}

func ptzSource(source string) (string, error) {
	sources := []string{source}
	if stream := streams.Get(source); stream != nil {
		sources = stream.Sources()
	}
	for _, rawURL := range sources {
		rawURL, _ = extractFragment(rawURL)
		u, err := url.Parse(rawURL)
		if err == nil && u.Scheme == "onvif" && u.Hostname() != "" {
			return u.String(), nil
		}
	}
	return "", errors.New("onvif: source must contain an onvif:// camera URL")
}

func lockPTZSession(source string) *ptzSession {
	ptzSessions.Lock()
	// Drop idle entries, including old credentials after runtime config changes.
	if time.Now().After(ptzSessions.nextSweep) {
		ptzSessions.nextSweep = time.Now().Add(time.Minute)
		for key, session := range ptzSessions.items {
			if session.users == 0 && session.mu.TryLock() {
				if session.timer == nil && time.Since(session.lastUsed) > 5*time.Minute {
					delete(ptzSessions.items, key)
				}
				session.mu.Unlock()
			}
		}
	}
	session := ptzSessions.items[source]
	if session == nil {
		session = new(ptzSession)
		ptzSessions.items[source] = session
	}
	session.users++
	session.lastUsed = time.Now()
	ptzSessions.Unlock()
	session.mu.Lock()
	return session
}

func unlockPTZSession(session *ptzSession) {
	session.mu.Unlock()
	ptzSessions.Lock()
	session.users--
	session.lastUsed = time.Now()
	ptzSessions.Unlock()
}

// Reuse the existing camera session. It can be discarded after a failed move.
func (s *ptzSession) ensureClient(source string) error {
	if s.client != nil {
		return nil
	}
	client, err := onvif.NewClientWithTimeout(source, 2*time.Second)
	if err != nil {
		return err
	}
	token, err := client.PTZProfile()
	if err != nil {
		return err
	}
	s.client = client
	s.token = token
	return nil
}

func apiPTZ(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var cmd ptzCommand
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		http.Error(w, "Invalid PTZ command", http.StatusBadRequest)
		return
	}
	if decoder.Decode(new(any)) != io.EOF || cmd.Source == "" ||
		(cmd.Action != "move" && cmd.Action != "stop" && cmd.Action != "presets" && cmd.Action != "goto-preset") {
		http.Error(w, "Invalid PTZ command", http.StatusBadRequest)
		return
	}
	if cmd.Action == "goto-preset" && cmd.Preset == "" {
		http.Error(w, "Missing preset token", http.StatusBadRequest)
		return
	}
	source, err := ptzSource(cmd.Source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	session := lockPTZSession(source)
	defer unlockPTZSession(session)
	if err = session.ensureClient(source); err != nil {
		http.Error(w, "Failed to initialize PTZ camera", http.StatusBadGateway)
		return
	}
	switch cmd.Action {
	case "move":
		_, err = session.client.PTZRequest(onvif.PTZContinuousMove, session.token,
			formatSpeed(cmd.Pan), formatSpeed(cmd.Tilt), formatSpeed(cmd.Zoom))
		if err != nil {
			session.discard = true
		}
		// A failed response does not prove that the camera did not start moving.
		session.armStop()
	case "stop":
		err = session.stop()
	case "presets":
		// Read-only: does not change active movement or its safety timer.
		var presets []onvif.PTZPreset
		presets, err = session.client.PTZPresets(session.token)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(presets)
			return
		}
	case "goto-preset":
		// A delayed safety Stop must not interrupt travel to the preset.
		if session.timer != nil {
			if err = session.stop(); err != nil {
				break
			}
			// stop() can discard the client after a previous failed move.
			if err = session.ensureClient(source); err != nil {
				break
			}
		}
		_, err = session.client.PTZRequest(onvif.PTZGotoPreset, session.token, cmd.Preset)
	}
	if err != nil {
		// Do not expose camera URLs or credentials in errors.
		http.Error(w, "PTZ command failed", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *ptzSession) armStop() {
	s.deadline = time.Now().Add(ptzLease)
	if s.timer != nil {
		s.timer.Reset(ptzLease)
	} else {
		s.timer = time.AfterFunc(ptzLease, s.expire)
	}
}

func (s *ptzSession) expire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A timer that already fired may be waiting behind a newer move or stop.
	if s.timer == nil || time.Now().Before(s.deadline) {
		return
	}
	_ = s.stop()
}

func (s *ptzSession) stop() error {
	_, err := s.client.PTZRequest(onvif.PTZStop, s.token)
	if err != nil {
		// Retry independently of the browser, including after it closes.
		s.armStop()
		return err
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.discard {
		s.client = nil
		s.token = ""
		s.discard = false
	}
	return nil
}

func formatSpeed(speed float64) string {
	if speed < -1 {
		speed = -1
	} else if speed > 1 {
		speed = 1
	}
	return fmt.Sprintf("%.2f", speed)
}
