package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dotvezz/smolid"
)

const baseDir = "/srv"

func handleRequest(w http.ResponseWriter, r *http.Request) {
	reqID := smolid.New()
	w.Header().Set("X-Origin-Request-Id", reqID.String())

	cleanPath := filepath.Clean(r.URL.Path)
	targetDir := filepath.Join(baseDir, r.Method, cleanPath)

	info, err := os.Stat(targetDir)
	if err != nil || !info.IsDir() {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Optional delay
	delayFile := filepath.Join(targetDir, "delay")
	if dBytes, err := os.ReadFile(delayFile); err == nil {
		if ms, err := strconv.Atoi(strings.TrimSpace(string(dBytes))); err == nil && ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}

	// Status
	statusCode := http.StatusOK
	statusFile := filepath.Join(targetDir, "status")
	if sBytes, err := os.ReadFile(statusFile); err == nil {
		if code, err := strconv.Atoi(strings.TrimSpace(string(sBytes))); err == nil {
			statusCode = code
		}
	}

	// Headers
	etagValue := ""
	headersFile := filepath.Join(targetDir, "headers")
	if hFile, err := os.Open(headersFile); err == nil {
		scanner := bufio.NewScanner(hFile)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, v, ok := strings.Cut(line, ":"); ok {
				k = strings.TrimSpace(k)
				v = strings.TrimSpace(v)
				w.Header().Add(k, v)
				if strings.EqualFold(k, "ETag") {
					etagValue = v
				}
			}
		}
		_ = hFile.Close()
	}

	// ETag matching for conditional requests
	if etagValue != "" {
		ifNoneMatch := r.Header.Get("If-None-Match")
		if ifNoneMatch != "" && (ifNoneMatch == etagValue || strings.Contains(ifNoneMatch, etagValue)) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	// Write status header
	w.WriteHeader(statusCode)

	// Body
	bodyFile := filepath.Join(targetDir, "body")
	if bodyBytes, err := os.ReadFile(bodyFile); err == nil {
		_, _ = w.Write(bodyBytes)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", handleRequest)
	log.Printf("Mock origin listening on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
