package mock_origin

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var baseDir string

func init() {
	if baseDir = os.Getenv("MOCK_ORIGIN_BASE_DIR"); baseDir == "" {
		baseDir = "./_test/mock-responses"
	}
}

var fileCache sync.Map

type MockOrigin struct {
	counter int
}

func (m *MockOrigin) IncrementCount() {
	m.counter++
}

func (m *MockOrigin) ResetCount() {
	m.counter = 0
}

func (m *MockOrigin) RequestCount() int {
	return m.counter
}

func (m *MockOrigin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.IncrementCount()

	cleanPath := filepath.Clean(r.URL.Path)
	wd, _ := os.Getwd()
	targetDir := filepath.Join(wd, baseDir, r.Method, cleanPath)

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
			http.Error(w, "Not Found", http.StatusNotFound)
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
