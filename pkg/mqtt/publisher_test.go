package mqtt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rossigee/fluent-bit-amqp-plugin/pkg/config"
)

func mqttConfig() *config.AMQPConfig {
	cfg := config.DefaultConfig()
	cfg.Backend = config.BackendMQTT
	// Connection refused on a closed port fails immediately, so the tests do not wait for timeouts.
	cfg.MQTT.Broker = "tcp://127.0.0.1:1"
	return cfg
}

func TestNewPublisher_RejectsInvalidConfig(t *testing.T) {
	cfg := mqttConfig()
	cfg.MQTT.QoS = 9
	if _, err := NewPublisher(cfg); err == nil {
		t.Fatal("expected NewPublisher to reject qos 9")
	}
}

func TestPublishRecord_UnreachableBroker(t *testing.T) {
	p, err := NewPublisher(mqttConfig())
	if err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}
	err = p.PublishRecord(time.Now(), map[string]interface{}{"message": "x"}, "tag")
	if err == nil || !strings.Contains(err.Error(), "failed to connect to MQTT broker") {
		t.Fatalf("PublishRecord() error = %v, want a connection error", err)
	}
}

func TestClose_NeverConnected(t *testing.T) {
	p, err := NewPublisher(mqttConfig())
	if err != nil {
		t.Fatalf("NewPublisher() error = %v", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("Close() on a never-connected publisher error = %v", err)
	}
}

func TestBuildTLSConfig_MissingCAFile(t *testing.T) {
	m := config.DefaultConfig().MQTT
	m.TLSCACert = filepath.Join(t.TempDir(), "missing.pem")
	if _, err := buildTLSConfig(&m); err == nil || !strings.Contains(err.Error(), "failed to read CA certificate") {
		t.Fatalf("buildTLSConfig() error = %v, want CA read error", err)
	}
}

func TestBuildTLSConfig_InvalidCAFile(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatalf("writing CA file: %v", err)
	}
	m := config.DefaultConfig().MQTT
	m.TLSCACert = caPath
	if _, err := buildTLSConfig(&m); err == nil || !strings.Contains(err.Error(), "failed to parse CA certificate") {
		t.Fatalf("buildTLSConfig() error = %v, want CA parse error", err)
	}
}

func TestBuildTLSConfig_CAAndClientCert(t *testing.T) {
	dir := t.TempDir()
	caPath, certPath, keyPath := writeTestPKI(t, dir)

	m := config.DefaultConfig().MQTT
	m.TLSCACert = caPath
	m.TLSClientCert = certPath
	m.TLSClientKey = keyPath
	tlsConfig, err := buildTLSConfig(&m)
	if err != nil {
		t.Fatalf("buildTLSConfig() error = %v", err)
	}
	if tlsConfig.RootCAs == nil {
		t.Error("expected RootCAs from the CA file")
	}
	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("expected 1 client certificate, got %d", len(tlsConfig.Certificates))
	}
	if tlsConfig.InsecureSkipVerify {
		t.Error("server certificate verification must stay enabled")
	}
}

// writeTestPKI writes a short-lived ECDSA P-256 certificate, used as its own CA, and a
// client key pair into dir. It exists only for these tests and is removed with dir.
func writeTestPKI(t *testing.T, dir string) (caPath, certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "unit-test"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshalling key: %v", err)
	}
	caPath = filepath.Join(dir, "ca.pem")
	certPath = filepath.Join(dir, "client.pem")
	keyPath = filepath.Join(dir, "client.key")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatalf("writing CA file: %v", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("writing cert file: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("writing key file: %v", err)
	}
	return caPath, certPath, keyPath
}
