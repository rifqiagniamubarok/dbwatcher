package markerapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rifqiagniamubarok/dbwatcher/internal/store"
)

// maxLabelLen caps the inbound marker label / log message length. Keeps the
// TUI render predictable and prevents a noisy client from spamming huge
// strings into the ring buffer.
const maxLabelLen = 200

// maxBodyBytes caps the request body to discourage abuse. 4 KiB easily
// covers maxLabelLen even with full JSON envelope overhead.
const maxBodyBytes = 4 * 1024

type handlers struct {
	store   Pusher
	startAt time.Time
	version string
}

type markerJSON struct {
	Label string `json:"label"`
	Color string `json:"color"`
}

type logJSON struct {
	Message string `json:"message"`
}

type healthJSON struct {
	Status   string `json:"status"`
	Uptime   int64  `json:"uptime_s"`
	Version  string `json:"version,omitempty"`
}

func (h *handlers) handleMarker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	label, color, err := parseMarkerBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.store.Push(store.NewMarker(label, color))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	message, err := parseLogBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.store.Push(store.NewLog(message))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}

	writeJSON(w, http.StatusOK, healthJSON{
		Status:  "ok",
		Uptime:  int64(time.Since(h.startAt).Seconds()),
		Version: h.version,
	})
}

func parseMarkerBody(r *http.Request) (label, color string, err error) {
	body, err := readBody(r)
	if err != nil {
		return "", "", err
	}

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		var m markerJSON
		if err := json.Unmarshal(body, &m); err != nil {
			return "", "", fmt.Errorf("invalid JSON: %w", err)
		}
		label = m.Label
		color = m.Color
	} else {
		// text/plain or unspecified: body is the label.
		label = string(body)
	}

	label = strings.TrimSpace(label)
	if label == "" {
		return "", "", errors.New("label is required")
	}
	if len(label) > maxLabelLen {
		return "", "", fmt.Errorf("label exceeds %d characters", maxLabelLen)
	}

	color = strings.ToLower(strings.TrimSpace(color))
	if color == "" {
		color = store.MarkerColorDefault
	}
	if !isAllowedColor(color) {
		return "", "", fmt.Errorf("color %q not allowed (allowed: %s)", color, strings.Join(store.AllowedMarkerColors, ", "))
	}
	return label, color, nil
}

func parseLogBody(r *http.Request) (string, error) {
	body, err := readBody(r)
	if err != nil {
		return "", err
	}

	contentType := r.Header.Get("Content-Type")
	var message string
	if strings.HasPrefix(contentType, "application/json") {
		var m logJSON
		if err := json.Unmarshal(body, &m); err != nil {
			return "", fmt.Errorf("invalid JSON: %w", err)
		}
		message = m.Message
	} else {
		message = string(body)
	}

	message = strings.TrimRight(message, "\r\n")
	if strings.TrimSpace(message) == "" {
		return "", errors.New("message is required")
	}
	if len(message) > maxLabelLen {
		return "", fmt.Errorf("message exceeds %d characters", maxLabelLen)
	}
	return message, nil
}

func readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(body) > maxBodyBytes {
		return nil, fmt.Errorf("body exceeds %d bytes", maxBodyBytes)
	}
	return body, nil
}

func isAllowedColor(c string) bool {
	for _, allowed := range store.AllowedMarkerColors {
		if c == allowed {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
