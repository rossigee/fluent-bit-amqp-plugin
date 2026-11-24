#!/usr/bin/env bash
# Runs the TLS integration tests against throwaway, TLS-enabled brokers: a
# mutual-TLS RabbitMQ (AMQPS) and a mutual-TLS Mosquitto (MQTT over ssl://).
#
# For localhost testing only. A test CA, a broker certificate and a client
# certificate are generated (ECDSA P-256) into a temporary directory for each
# run, and the directory and containers are removed on exit. Nothing is written
# to the repo.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
AMQP_IMAGE="rabbitmq:4.3.5-management-alpine"
AMQP_CONTAINER="amqp-tls-integration"
MQTT_IMAGE="eclipse-mosquitto:2"
MQTT_CONTAINER="amqp-tls-mqtt-integration"
TLS_USER="tls-test"
TLS_PASS="tls-test"
CERT_DIR="$(mktemp -d)"
BROKER_DIR="${CERT_DIR}/broker"
AMQP_STARTED=0
MQTT_STARTED=0

cleanup() {
	if [ "${AMQP_STARTED}" -eq 1 ]; then
		docker rm -f "${AMQP_CONTAINER}" >/dev/null
	fi
	if [ "${MQTT_STARTED}" -eq 1 ]; then
		docker rm -f "${MQTT_CONTAINER}" >/dev/null
	fi
	rm -rf "${CERT_DIR}"
}
trap cleanup EXIT

# issue NAME CN EXT_USAGE SAN_LINE
# Creates an ECDSA key and a certificate for it, signed by the test CA.
issue() {
	local name="$1" cn="$2" usage="$3" san="$4"
	openssl ecparam -name prime256v1 -genkey -noout -out "${CERT_DIR}/${name}.key"
	openssl req -new -key "${CERT_DIR}/${name}.key" -subj "/CN=${cn}" -out "${CERT_DIR}/${name}.csr"
	{
		printf 'basicConstraints=CA:FALSE\n'
		printf 'keyUsage=digitalSignature\n'
		printf 'extendedKeyUsage=%s\n' "${usage}"
		if [ -n "${san}" ]; then
			printf '%s\n' "${san}"
		fi
	} > "${CERT_DIR}/${name}.ext"
	openssl x509 -req -in "${CERT_DIR}/${name}.csr" \
		-CA "${CERT_DIR}/ca.pem" -CAkey "${CERT_DIR}/ca.key" -CAcreateserial \
		-days 2 -sha256 -extfile "${CERT_DIR}/${name}.ext" \
		-out "${CERT_DIR}/${name}.pem"
}

openssl ecparam -name prime256v1 -genkey -noout -out "${CERT_DIR}/ca.key"
openssl req -x509 -new -key "${CERT_DIR}/ca.key" -sha256 -days 2 \
	-subj "/CN=amqp-plugin-test-ca" \
	-addext "basicConstraints=critical,CA:TRUE" \
	-addext "keyUsage=critical,keyCertSign,cRLSign" \
	-out "${CERT_DIR}/ca.pem"

issue server "amqp-plugin-test-broker" serverAuth "subjectAltName=DNS:localhost,IP:127.0.0.1"
issue client "amqp-plugin-test-client" clientAuth ""

# The brokers run as their own users inside the containers, so the mounted files must be readable.
mkdir -m 0755 "${BROKER_DIR}"
cp "${CERT_DIR}/ca.pem" "${CERT_DIR}/server.pem" "${CERT_DIR}/server.key" "${BROKER_DIR}/"
chmod 0644 "${BROKER_DIR}/ca.pem" "${BROKER_DIR}/server.pem" "${BROKER_DIR}/server.key"
chmod 0644 "${CERT_DIR}/client.pem" "${CERT_DIR}/client.key"

# AMQPS: RabbitMQ requiring client certificates.
AMQP_STARTED=1
docker run -d --name "${AMQP_CONTAINER}" -p 5671:5671 \
	-v "${BROKER_DIR}:/certs:ro" \
	-v "${SCRIPT_DIR}/rabbitmq-tls.conf:/etc/rabbitmq/rabbitmq.conf:ro" \
	-e "RABBITMQ_DEFAULT_USER=${TLS_USER}" \
	-e "RABBITMQ_DEFAULT_PASS=${TLS_PASS}" \
	"${AMQP_IMAGE}" >/dev/null

# MQTTS: Mosquitto requiring client certificates on port 8883.
MQTT_STARTED=1
docker run -d --name "${MQTT_CONTAINER}" -p 8883:8883 \
	-v "${BROKER_DIR}:/certs:ro" \
	-v "${SCRIPT_DIR}/mosquitto-tls.conf:/mosquitto/config/mosquitto.conf:ro" \
	"${MQTT_IMAGE}" >/dev/null

# wait_for_log CONTAINER PATTERN
# Polls the container's log until PATTERN appears. Each check is a full snapshot of the
# log, so the wait ends as soon as the broker reports ready. It fails after a fixed number
# of attempts rather than waiting forever.
wait_for_log() {
	local container="$1" pattern="$2" attempts=0 logs
	while :; do
		logs="$(docker logs "${container}" 2>&1)"
		if [[ "${logs}" == *"${pattern}"* ]]; then
			return 0
		fi
		attempts=$((attempts + 1))
		if [ "${attempts}" -ge 600 ]; then
			echo "timed out waiting for ${container} to report: ${pattern}" >&2
			return 1
		fi
	done
}

wait_for_log "${AMQP_CONTAINER}" "Server startup complete"
wait_for_log "${MQTT_CONTAINER}" "running"

export RABBITMQ_TLS_URL="amqps://${TLS_USER}:${TLS_PASS}@localhost:5671/"
export RABBITMQ_TLS_CA_FILE="${CERT_DIR}/ca.pem"
export RABBITMQ_TLS_CERT_FILE="${CERT_DIR}/client.pem"
export RABBITMQ_TLS_KEY_FILE="${CERT_DIR}/client.key"
export MQTT_TLS_URL="ssl://localhost:8883"
export MQTT_TLS_CA_FILE="${CERT_DIR}/ca.pem"
export MQTT_TLS_CERT_FILE="${CERT_DIR}/client.pem"
export MQTT_TLS_KEY_FILE="${CERT_DIR}/client.key"

cd "${REPO_DIR}"
go test -count=1 -tags integration -run 'TestPublishTLS|TestMQTTTLS' -v ./test/integration/
