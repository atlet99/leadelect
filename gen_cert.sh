
#!/bin/bash

# https://gist.github.com/NovikovRoman/30302d68a661526dfeddd503da39e8f9

# Configuration variables
IP=127.0.0.1
DNS=DNS:localhost # Additional DNS entries can be added, e.g., DNS:*.tls,DNS:0.0.0.0
CA_DAYS=365
SERVER_CERT_DAYS=365
CERTS_DIR="certs"

# Check if openssl is installed
if ! command -v openssl &> /dev/null; then
    echo "Error: OpenSSL is not installed."
    exit 1
fi

# Set -e to exit the script if any command fails
set -e

# Create certs directory if it doesn't exist
mkdir -p ${CERTS_DIR} && cd ${CERTS_DIR}

# Clean up old certificates if they exist
rm -f *.pem *.cnf

# Generate CA's private key and self-signed certificate
echo "Generating CA's private key and self-signed certificate..."
openssl req -x509 -newkey rsa:4096 -days ${CA_DAYS} -nodes \
    -keyout ca-key.pem -out ca-cert.pem -subj "/CN=Certificate authority/" 2>/dev/null

# Display CA certificate info (optional)
openssl x509 -in ca-cert.pem -noout

# Generate server's private key and certificate signing request (CSR)
echo "Generating web server's private key and CSR..."
openssl req -newkey rsa:4096 -nodes \
    -keyout server-key.pem -out server-req.pem -subj "/CN=Server/" 2>/dev/null

# Create SAN extension configuration
echo "subjectAltName=${DNS},IP:${IP}" > server-ext.cnf

# Sign server's CSR with CA's private key to generate the server certificate
echo "Signing server's CSR with CA's private key to create the server certificate..."
openssl x509 -req -in server-req.pem -days ${SERVER_CERT_DAYS} \
    -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
    -out server-cert.pem -extfile server-ext.cnf

echo "Certificates generated successfully in the ${CERTS_DIR} directory."
