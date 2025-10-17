#!/bin/bash
# Railway install script to skip native module builds
# These modules are only needed for local development and testing

echo "Installing dependencies for Railway production build..."
pnpm install --frozen-lockfile --ignore-scripts

echo "Dependencies installed successfully!"