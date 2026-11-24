#!/usr/bin/env bash
# Runs the TLS integration tests against a throwaway, TLS-enabled RabbitMQ.
#
# For localhost testing only. A test CA, a broker certificate and a client
# certificate are generated (ECDSA P-256) into a temporary directory for each
# run, and the directory is removed on exit. Nothing is written to the repo.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
IMAGE="rabbitmq:4.3.5-management-alpine"
CONTAINER="amqp-tls-integration"
TLS_USER="tls-test"
TLS_PASS="tls-test"
CERT_DIR="$(mktemp -d)"
BROKER_DIR="${CERT_DIR}/broker"
CONTAINER_STARTED=0

cleanup() {
	if [ "${CONTAINER_STARTED}" -eq 1 ]; then
		docker rm -f "${CONTAINER}" >/dev/null
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

# The broker runs as its own user inside the container, so the mounted files must be readable.
mkdir "${BROKER_DIR}"
cp "${CERT_DIR}/ca.pem" "${CERT_DIR}/server.pem" "${BROKER_DIR}/"
cp "${CERT_DIR}/server.key" "${BROKER_DIR}/"
chmod 0644 "${BROKER_DIR}/server.key"
chmod 0644 "${CERT_DIR}/client.key"

CONTAINER_STARTED=1
docker run -d --name "${CONTAINER}" -p 5671:5671 \
	-v "${BROKER_DIR}:/certs:ro" \
	-v "${SCRIPT_DIR}/rabbitmq-tls.conf:/etc/rabbitmq/rabbitmq.conf:ro" \
	-e "RABBITMQ_DEFAULT_USER=${TLS_USER}" \
	-e "RABBITMQ_DEFAULT_PASS=${TLS_PASS}" \
	"${IMAGE}" >/dev/null

# Follow the broker log until it reports startup complete; the timeout fails the script if it never does.
# shellcheck disable=SC2016 # $1 must expand in the inner shell, not here
timeout 180 sh -c 'docker logs -f "$1" 2>&1 | grep -q -m1 "Server startup complete"' _ "${CONTAINER}"

export RABBITMQ_TLS_URL="amqps://${TLS_USER}:${TLS_PASS}@localhost:5671/"
export RABBITMQ_TLS_CA_FILE="${CERT_DIR}/ca.pem"
export RABBITMQ_TLS_CERT_FILE="${CERT_DIR}/client.pem"
export RABBITMQ_TLS_KEY_FILE="${CERT_DIR}/client.key"

cd "${REPO_DIR}"
go test -count=1 -tags integration -run 'TestPublishTLS' -v ./test/integration/
