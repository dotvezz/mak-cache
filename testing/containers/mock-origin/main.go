package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dotvezz/smolid"
)

const baseDir = "/srv"

var requestCount uint64
var fileCache sync.Map

func getFileBytes(path string) ([]byte, error) {
	if v, ok := fileCache.Load(path); ok {
		return v.([]byte), nil
	}
	data, err := os.ReadFile(path)
	if err == nil {
		fileCache.Store(path, data)
	}
	return data, err
}

func handleCounter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	switch r.Method {
	case http.MethodGet:
		count := atomic.LoadUint64(&requestCount)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"count":%d}`, count)
	case http.MethodDelete:
		atomic.StoreUint64(&requestCount, 0)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"count":0}`)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)

	reqID := smolid.New()
	w.Header().Set("X-Origin-Request-Id", reqID.String())

	cleanPath := filepath.Clean(r.URL.Path)
	targetDir := filepath.Join(baseDir, r.Method, cleanPath)

	info, err := os.Stat(targetDir)
	if err != nil && !strings.HasPrefix(cleanPath, "/file") {
		altDir := filepath.Join(baseDir, r.Method, "file", cleanPath)
		if altInfo, altErr := os.Stat(altDir); altErr == nil {
			targetDir = altDir
			info = altInfo
			err = nil
		}
	}

	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if !info.IsDir() {
		bodyBytes, err := getFileBytes(targetDir)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(bodyBytes)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bodyBytes)
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

	http.HandleFunc("/requests", handleCounter)
	http.HandleFunc("/_requests", handleCounter)
	http.HandleFunc("/count", handleCounter)
	http.HandleFunc("/_count", handleCounter)
	http.HandleFunc("/", handleRequest)
	log.Printf("Mock origin listening on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
