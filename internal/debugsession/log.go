package debugsession

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

const logPath = "debug-b03873.log"

var logMu sync.Mutex

// Log appends one NDJSON debug line for session b03873.
func Log(hypothesisID, location, message string, data map[string]any) {
	entry := map[string]any{
		"sessionId":    "b03873",
		"hypothesisId": hypothesisID,
		"location":     location,
		"message":      message,
		"timestamp":    time.Now().UnixMilli(),
	}
	if data != nil {
		entry["data"] = data
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(raw, '\n'))
	_ = f.Close()
}
