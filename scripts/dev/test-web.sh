#!/usr/bin/env sh
set -eu

npm ci
npm test
npm run build
npm audit --audit-level=high
