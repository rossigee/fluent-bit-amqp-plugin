package amqp

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
)

func TestNewPublisher_NoConnection(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.URL = "amqp://invalid-host-that-does-not-exist:5672/"

	publisher, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("Expected no error during NewPublisher, got: %v", err)
	}

	if publisher.connection != nil {
		t.Error("Expected connection to be nil (lazy connection)")
	}
	if publisher.channel != nil {
		t.Error("Expected channel to be nil (lazy connection)")
	}
}

func TestPublisher_CloseNeverConnected(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.URL = "amqp://invalid-host-that-does-not-exist:5672/"

	publisher, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("Expected no error during NewPublisher, got: %v", err)
	}

	err = publisher.Close()
	if err != nil {
		t.Fatalf("Expected no error when closing never-connected publisher, got: %v", err)
	}

	if publisher.connection != nil {
		t.Error("Expected connection to be nil after close")
	}
	if publisher.channel != nil {
		t.Error("Expected channel to be nil after close")
	}
}

func TestPublisher_EnsureConnection_InvalidHost(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.URL = "amqp://invalid-host-that-does-not-exist:5672/"

	publisher, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("Expected no error during NewPublisher, got: %v", err)
	}

	err = publisher.ensureConnection()
	if err == nil {
		t.Error("Expected error when connecting to invalid host")
	}
}

func TestPublishRecord_InvalidHost(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.URL = "amqp://invalid-host-that-does-not-exist:5672/"

	publisher, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("Expected no error during NewPublisher, got: %v", err)
	}

	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	record := map[string]interface{}{"message": "hello"}
	err = publisher.PublishRecord(ts, record, "test.tag")
	if err == nil {
		t.Error("Expected error when publishing to unreachable host")
	}
}

func TestNormalizeRecord_StringKeys(t *testing.T) {
	input := map[string]interface{}{"a": 1, "b": "two"}
	result := normalizeRecord(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}
	if m["a"] != 1 || m["b"] != "two" {
		t.Errorf("unexpected map contents: %v", m)
	}
}

func TestNormalizeRecord_InterfaceKeys(t *testing.T) {
	input := map[interface{}]interface{}{
		"msg":    "hello",
		"level":  "info",
		"nested": map[interface{}]interface{}{"k": "v"},
	}
	result := normalizeRecord(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}
	if m["msg"] != "hello" {
		t.Errorf("expected msg=hello, got %v", m)
	}
	nested, ok := m["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested map[string]interface{}, got %T", m["nested"])
	}
	if nested["k"] != "v" {
		t.Errorf("expected nested k=v, got %v", nested)
	}
}

func TestNormalizeRecord_Slice(t *testing.T) {
	input := []interface{}{
		map[interface{}]interface{}{"x": 1},
		"plain",
	}
	result := normalizeRecord(input)
	s, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result)
	}
	if len(s) != 2 {
		t.Fatalf("expected length 2, got %d", len(s))
	}
	m, ok := s[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{} in slice, got %T", s[0])
	}
	if m["x"] != 1 {
		t.Errorf("expected x=1, got %v", m)
	}
}

func TestEncodeURLPassword(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantURL string
	}{
		{
			name:    "no password",
			rawURL:  "amqp://guest@localhost:5672/",
			wantURL: "amqp://guest@localhost:5672/",
		},
		{
			name:    "simple password",
			rawURL:  "amqp://guest:password@localhost:5672/",
			wantURL: "amqp://guest:password@localhost:5672/",
		},
		{
			name:    "password with plus",
			rawURL:  "amqp://user:QR+bllqd3Ikt@localhost:5672/",
			wantURL: "amqp://user:QR%2Bbllqd3Ikt@localhost:5672/",
		},
		{
			name:    "password with equals",
			rawURL:  "amqp://user:abc=123@localhost:5672/",
			wantURL: "amqp://user:abc%3D123@localhost:5672/",
		},
		{
			name:    "complex password like vault",
			rawURL:  "amqp://fluentbit:QR+bllqd3IktDyX9FolFT5+Aq+8Fdv9A@queues.bankrut.lan:5672/system",
			wantURL: "amqp://fluentbit:QR%2Bbllqd3IktDyX9FolFT5%2BAq%2B8Fdv9A@queues.bankrut.lan:5672/system",
		},
		{
			name:    "already encoded password - should not double encode",
			rawURL:  "amqps://fluentbit:Fz%2FZC7XqFnb%2F9uGZkjiprNqbR0HgHIjc@queues.bankrut.lan:5671/system",
			wantURL: "amqps://fluentbit:Fz%2FZC7XqFnb%2F9uGZkjiprNqbR0HgHIjc@queues.bankrut.lan:5671/system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeURLPassword(tt.rawURL)
			if err != nil {
				t.Errorf("encodeURLPassword() error = %v", err)
				return
			}
			if got != tt.wantURL {
				t.Errorf("encodeURLPassword() = %v, want %v", got, tt.wantURL)
			}
		})
	}
}

func TestBuildTLSConfig_NoFilesConfigured(t *testing.T) {
	p := &Publisher{config: config.DefaultConfig()}

	tlsConfig, err := p.buildTLSConfig()
	if err != nil {
		t.Fatalf("buildTLSConfig() error = %v", err)
	}
	if len(tlsConfig.Certificates) != 0 {
		t.Errorf("expected no client certificates, got %d", len(tlsConfig.Certificates))
	}
	if tlsConfig.RootCAs != nil {
		t.Error("expected RootCAs to be nil so system roots are used")
	}
}

func TestBuildTLSConfig_MissingCAFile(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TLSCAFile = filepath.Join(t.TempDir(), "does-not-exist.pem")
	p := &Publisher{config: cfg}

	_, err := p.buildTLSConfig()
	if err == nil || !strings.Contains(err.Error(), "failed to read CA certificate") {
		t.Fatalf("expected CA read error, got %v", err)
	}
}

func TestBuildTLSConfig_InvalidCAFile(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, []byte("not a PEM certificate\n"), 0o600); err != nil {
		t.Fatalf("writing CA file: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.TLSCAFile = caPath
	p := &Publisher{config: cfg}

	_, err := p.buildTLSConfig()
	if err == nil || !strings.Contains(err.Error(), "failed to parse CA certificate") {
		t.Fatalf("expected CA parse error, got %v", err)
	}
}

func TestBuildTLSConfig_MissingClientKeypair(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.TLSCertFile = filepath.Join(dir, "client.pem")
	cfg.TLSKeyFile = filepath.Join(dir, "client.key")
	p := &Publisher{config: cfg}

	_, err := p.buildTLSConfig()
	if err == nil || !strings.Contains(err.Error(), "failed to load client certificate") {
		t.Fatalf("expected client certificate load error, got %v", err)
	}
}

func TestBuildTLSConfig_InvalidClientKeypair(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "client.pem")
	keyPath := filepath.Join(dir, "client.key")
	if err := os.WriteFile(certPath, []byte("garbage\n"), 0o600); err != nil {
		t.Fatalf("writing cert file: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("garbage\n"), 0o600); err != nil {
		t.Fatalf("writing key file: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.TLSCertFile = certPath
	cfg.TLSKeyFile = keyPath
	p := &Publisher{config: cfg}

	_, err := p.buildTLSConfig()
	if err == nil || !strings.Contains(err.Error(), "failed to load client certificate") {
		t.Fatalf("expected client certificate load error, got %v", err)
	}
}

func TestBuildTLSConfig_ValidCAFile(t *testing.T) {
	// Reuse a CA certificate from the system trust store rather than generating
	// one, so no self-signed certificate is created by the test suite.
	bundle, err := os.ReadFile("/etc/ssl/certs/ca-certificates.crt")
	if err != nil {
		t.Skipf("system CA bundle not available: %v", err)
	}
	start := strings.Index(string(bundle), "-----BEGIN CERTIFICATE-----")
	end := strings.Index(string(bundle), "-----END CERTIFICATE-----")
	if start < 0 || end < 0 {
		t.Skip("no PEM certificate found in system CA bundle")
	}
	pemBlock := bundle[start : end+len("-----END CERTIFICATE-----")+1]

	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pemBlock, 0o600); err != nil {
		t.Fatalf("writing CA file: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.TLSCAFile = caPath
	p := &Publisher{config: cfg}

	tlsConfig, err := p.buildTLSConfig()
	if err != nil {
		t.Fatalf("buildTLSConfig() error = %v", err)
	}
	if tlsConfig.RootCAs == nil {
		t.Error("expected RootCAs to be set from CA file")
	}
	if tlsConfig.InsecureSkipVerify {
		t.Error("expected certificate verification to remain enabled")
	}
}

func TestReconnect_InvalidHost(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.URL = "amqp://invalid-host-that-does-not-exist:5672/"

	publisher, err := NewPublisher(cfg)
	if err != nil {
		t.Fatalf("Expected no error during NewPublisher, got: %v", err)
	}

	if err := publisher.reconnect(); err == nil {
		t.Error("Expected reconnect to fail when the host is unreachable")
	}
}

func TestIsValidEncodedPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{name: "no escape sequences", password: "abc123", want: true},
		{name: "valid escape sequence", password: "a%2Fb", want: true},
		{name: "lone percent sign", password: "100%sure", want: false},
		{name: "too short to hold an escape", password: "ab", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidEncodedPassword(tt.password); got != tt.want {
				t.Errorf("isValidEncodedPassword(%q) = %v, want %v", tt.password, got, tt.want)
			}
		})
	}
}

func TestQueueArguments(t *testing.T) {
	tests := []struct {
		name      string
		queueType string
		queueMode string
		want      amqp.Table
	}{
		{name: "classic default sends no arguments", queueType: "classic", queueMode: "default", want: nil},
		{name: "empty type and mode send no arguments", queueType: "", queueMode: "", want: nil},
		{name: "lazy classic", queueType: "classic", queueMode: "lazy", want: amqp.Table{"x-queue-mode": "lazy"}},
		{name: "quorum", queueType: "quorum", queueMode: "default", want: amqp.Table{"x-queue-type": "quorum"}},
		{name: "stream", queueType: "stream", queueMode: "default", want: amqp.Table{"x-queue-type": "stream"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.QueueType = tt.queueType
			cfg.QueueMode = tt.queueMode
			got := queueArguments(cfg)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("queueArguments() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessageProperties_Defaults(t *testing.T) {
	props := messageProperties(config.DefaultConfig())

	if props.DeliveryMode != amqp.Persistent {
		t.Errorf("DeliveryMode = %d, want %d (persistent)", props.DeliveryMode, amqp.Persistent)
	}
	if props.ContentType != "application/json" {
		t.Errorf("ContentType = %q, want application/json", props.ContentType)
	}
	if props.AppId != "fluent-bit" {
		t.Errorf("AppId = %q, want fluent-bit", props.AppId)
	}
	if props.Expiration != "" {
		t.Errorf("Expiration = %q, want unset when expiration is 0", props.Expiration)
	}
	if props.Priority != 0 {
		t.Errorf("Priority = %d, want 0", props.Priority)
	}
	if props.Type != "" {
		t.Errorf("Type = %q, want empty", props.Type)
	}
}

func TestMessageProperties_Overrides(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Persistent = false
	cfg.Expiration = 60000
	cfg.Priority = 5
	cfg.MessageType = "log"
	cfg.AppID = "edge-01"
	cfg.ContentType = "application/cloudevents+json"

	props := messageProperties(cfg)

	if props.DeliveryMode != amqp.Transient {
		t.Errorf("DeliveryMode = %d, want %d (transient)", props.DeliveryMode, amqp.Transient)
	}
	if props.Expiration != "60000" {
		t.Errorf("Expiration = %q, want 60000", props.Expiration)
	}
	if props.Priority != 5 {
		t.Errorf("Priority = %d, want 5", props.Priority)
	}
	if props.Type != "log" {
		t.Errorf("Type = %q, want log", props.Type)
	}
	if props.AppId != "edge-01" {
		t.Errorf("AppId = %q, want edge-01", props.AppId)
	}
	if props.ContentType != "application/cloudevents+json" {
		t.Errorf("ContentType = %q, want application/cloudevents+json", props.ContentType)
	}
}
