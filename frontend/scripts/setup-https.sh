#!/bin/bash

# Script to set up local HTTPS certificates using mkcert

echo "Setting up HTTPS certificates for local development..."

# Check if mkcert is installed
if ! command -v mkcert &> /dev/null; then
    echo "mkcert is not installed. Please install it first:"
    echo ""
    echo "On Ubuntu/Debian:"
    echo "  sudo apt install libnss3-tools"
    echo "  curl -JLO \"https://dl.filippo.io/mkcert/latest?for=linux/amd64\""
    echo "  chmod +x mkcert-v*-linux-amd64"
    echo "  sudo cp mkcert-v*-linux-amd64 /usr/local/bin/mkcert"
    echo ""
    echo "On macOS:"
    echo "  brew install mkcert"
    echo ""
    exit 1
fi

# Create certificate directory
mkdir -p .cert

# Install local CA
mkcert -install

# Generate certificates for localhost and local IP
echo "Generating certificates..."
cd .cert
mkcert localhost 127.0.0.1 ::1 192.168.50.203 mssb.local

# Rename to expected names (glob handles different domain counts)
mv localhost+*.pem localhost.pem
mv localhost+*-key.pem localhost-key.pem

cd ..

echo "✅ HTTPS certificates created successfully!"
echo ""
echo "The certificates are stored in ./.cert/"
echo "You can now run 'pnpm dev' and access the app via HTTPS"