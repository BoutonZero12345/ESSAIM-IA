package utils

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
)

// InitLogger configures the global logger with timestamps and file/line info.
// Logs go to both stdout and logs/essaim.log.
func InitLogger() {
	// Create logs directory
	logDir := "logs"
	os.MkdirAll(logDir, 0755)

	logPath := filepath.Join(logDir, "essaim.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
		log.SetOutput(os.Stdout)
		log.Printf("[LOGGER] ⚠ Could not create log file: %v (stdout only)", err)
		return
	}

	// Multi-writer: stdout + file
	multi := io.MultiWriter(os.Stdout, logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.SetOutput(multi)
	log.Println("[LOGGER] Initialized (stdout + " + logPath + ")")
}

// ParseJSON safely unmarshals a JSON string into the target struct.
func ParseJSON(raw string, target interface{}) error {
	return json.Unmarshal([]byte(raw), target)
}

// ToJSON marshals any struct to a JSON string.
func ToJSON(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetEnv reads an environment variable with a fallback default.
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
