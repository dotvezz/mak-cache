package config

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestTimingConfig_JSON(t *testing.T) {
	t.Run("unmarshal duration strings", func(t *testing.T) {
		jsonData := []byte(`{
			"ttl": "10s",
			"max_stale": "5m",
			"ttl_splay": "500ms"
		}`)

		var timing TimingConfig
		err := json.Unmarshal(jsonData, &timing)
		if err != nil {
			t.Fatalf("unexpected unmarshal error: %v", err)
		}

		expected := TimingConfig{
			TTL:      10 * time.Second,
			MaxStale: 5 * time.Minute,
			TTLSplay: 500 * time.Millisecond,
		}

		if timing != expected {
			t.Errorf("got TimingConfig %+v, want %+v", timing, expected)
		}
	})

	t.Run("unmarshal numeric durations", func(t *testing.T) {
		jsonData := []byte(`{
			"ttl": 10000000000
		}`)

		var timing TimingConfig
		err := json.Unmarshal(jsonData, &timing)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if timing.TTL != 10*time.Second {
			t.Errorf("got TTL %v, want 10s", timing.TTL)
		}
	})

	t.Run("marshal duration strings", func(t *testing.T) {
		timing := TimingConfig{
			TTL:      10 * time.Second,
			MaxStale: 5 * time.Minute,
		}

		data, err := json.Marshal(timing)
		if err != nil {
			t.Fatalf("unexpected marshal error: %v", err)
		}

		expectedJSON := `{"ttl":"10s","max_stale":"5m0s"}`
		if string(data) != expectedJSON {
			t.Errorf("got JSON %s, want %s", string(data), expectedJSON)
		}
	})

	t.Run("unmarshal invalid duration string error", func(t *testing.T) {
		jsonData := []byte(`{"ttl": "invalid_duration"}`)
		var timing TimingConfig
		err := json.Unmarshal(jsonData, &timing)
		if err == nil {
			t.Fatal("expected unmarshal error, got nil")
		}
	})

	t.Run("unmarshal unsupported type error", func(t *testing.T) {
		jsonData := []byte(`{"ttl": true}`)
		var timing TimingConfig
		err := json.Unmarshal(jsonData, &timing)
		if err == nil {
			t.Fatal("expected error for boolean duration, got nil")
		}
	})
}

func TestTimingConfig_YAML(t *testing.T) {
	t.Run("unmarshal YAML duration strings", func(t *testing.T) {
		yamlData := []byte(`
ttl: 10s
max_stale: 5m
ttl_splay: 1s
`)

		var timing TimingConfig
		err := yaml.Unmarshal(yamlData, &timing)
		if err != nil {
			t.Fatalf("unexpected YAML unmarshal error: %v", err)
		}

		expected := TimingConfig{
			TTL:      10 * time.Second,
			MaxStale: 5 * time.Minute,
			TTLSplay: 1 * time.Second,
		}

		if timing != expected {
			t.Errorf("got TimingConfig %+v, want %+v", timing, expected)
		}
	})

	t.Run("marshal YAML duration strings", func(t *testing.T) {
		timing := TimingConfig{
			TTL: 10 * time.Second,
		}

		out, err := yaml.Marshal(timing)
		if err != nil {
			t.Fatalf("unexpected YAML marshal error: %v", err)
		}

		expectedYAML := "ttl: 10s\n"
		if string(out) != expectedYAML {
			t.Errorf("got YAML %q, want %q", string(out), expectedYAML)
		}
	})
}

func TestRefreshConfig_JSONAndYAML(t *testing.T) {
	t.Run("JSON roundtrip", func(t *testing.T) {
		jsonData := []byte(`{
			"disable": true,
			"timeout": "500ms"
		}`)

		var ref RefreshConfig
		err := json.Unmarshal(jsonData, &ref)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !ref.Disable || ref.Timeout != 500*time.Millisecond {
			t.Errorf("got RefreshConfig %+v", ref)
		}

		data, err := json.Marshal(ref)
		if err != nil {
			t.Fatalf("unexpected marshal error: %v", err)
		}

		expectedJSON := `{"disable":true,"timeout":"500ms"}`
		if string(data) != expectedJSON {
			t.Errorf("got JSON %s, want %s", string(data), expectedJSON)
		}
	})

	t.Run("YAML roundtrip", func(t *testing.T) {
		yamlData := []byte(`
disable: false
timeout: 1s
`)

		var ref RefreshConfig
		err := yaml.Unmarshal(yamlData, &ref)
		if err != nil {
			t.Fatalf("unexpected YAML error: %v", err)
		}

		if ref.Timeout != time.Second {
			t.Errorf("got Timeout %v, want 1s", ref.Timeout)
		}

		out, err := yaml.Marshal(ref)
		if err != nil {
			t.Fatalf("unexpected YAML marshal error: %v", err)
		}

		expectedYAML := "disable: false\ntimeout: 1s\n"
		if string(out) != expectedYAML {
			t.Errorf("got YAML %q, want %q", string(out), expectedYAML)
		}
	})
}

func TestConfig_UnmarshalJSON(t *testing.T) {
	t.Run("status_timings map parsing", func(t *testing.T) {
		jsonData := []byte(`{
			"timing": {
				"ttl": "30s"
			},
			"status_timings": {
				"404": {
					"ttl": "10s"
				},
				"500": {
					"ttl": "1s"
				}
			}
		}`)

		var cfg Config
		err := json.Unmarshal(jsonData, &cfg)
		if err != nil {
			t.Fatalf("unexpected unmarshal error: %v", err)
		}

		if cfg.Timing.TTL != 30*time.Second {
			t.Errorf("Timing.TTL = %v, want 30s", cfg.Timing.TTL)
		}

		if len(cfg.StatusTimings) != 2 {
			t.Fatalf("expected 2 status timings, got %d", len(cfg.StatusTimings))
		}

		if cfg.StatusTimings[404].TTL != 10*time.Second {
			t.Errorf("StatusTimings[404].TTL = %v, want 10s", cfg.StatusTimings[404].TTL)
		}

		if cfg.StatusTimings[500].TTL != time.Second {
			t.Errorf("StatusTimings[500].TTL = %v, want 1s", cfg.StatusTimings[500].TTL)
		}
	})

	t.Run("invalid status_timings key error", func(t *testing.T) {
		jsonData := []byte(`{
			"status_timings": {
				"invalid_code": {
					"ttl": "10s"
				}
			}
		}`)

		var cfg Config
		err := json.Unmarshal(jsonData, &cfg)
		if err == nil {
			t.Fatal("expected error for invalid status code key, got nil")
		}
	})
}

func TestDefaultsResolvers(t *testing.T) {
	t.Run("ResolveComponents returns default if empty", func(t *testing.T) {
		keyCfg := CacheKeyConfig{}
		got := keyCfg.ResolveComponents()
		want := []string{"host", "path", "query"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ResolveComponents() = %v, want %v", got, want)
		}

		customCfg := CacheKeyConfig{Components: []string{"method", "path"}}
		gotCustom := customCfg.ResolveComponents()
		wantCustom := []string{"method", "path"}
		if !reflect.DeepEqual(gotCustom, wantCustom) {
			t.Errorf("ResolveComponents() = %v, want %v", gotCustom, wantCustom)
		}
	})

	t.Run("ResolveTimeout returns default if zero", func(t *testing.T) {
		refreshCfg := RefreshConfig{}
		got := refreshCfg.ResolveTimeout()
		if got != 30*time.Second {
			t.Errorf("ResolveTimeout() = %v, want 30s", got)
		}

		customRefresh := RefreshConfig{Timeout: 10 * time.Minute}
		gotCustom := customRefresh.ResolveTimeout()
		if gotCustom != 10*time.Minute {
			t.Errorf("ResolveTimeout() = %v, want 10m", gotCustom)
		}
	})
}
