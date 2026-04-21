#!/usr/bin/env bash
# Deploy the backend to Fly with git SHA + build time stamped into the
# binary. The running commit is exposed at /api/version and shown on the
# monitoring dashboard, so you can always tell what's in prod without
# leaving the admin UI.
#
# Usage: ./deploy.sh [extra fly-deploy flags...]
#   ./deploy.sh              # deploy current HEAD
#   ./deploy.sh --strategy=immediate
#
# Requires: fly CLI authenticated, clean or dirty working tree (a
# "-dirty" marker is appended if HEAD doesn't match the index).

set -euo pipefail

APP="${FLY_APP:-space-invaders-coop-backend}"

if ! command -v fly >/dev/null 2>&1; then
  echo "error: fly CLI not found on PATH" >&2
  exit 1
fi

if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo "error: not inside a git working tree" >&2
  exit 1
fi

GIT_SHA="$(git rev-parse --short=12 HEAD)"
if ! git diff --quiet HEAD -- || ! git diff --cached --quiet HEAD --; then
  GIT_SHA="${GIT_SHA}-dirty"
  echo "warning: deploying with uncommitted changes (${GIT_SHA})" >&2
fi

BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "==> Deploying ${APP}"
echo "    commit:     ${GIT_SHA}"
echo "    build_time: ${BUILD_TIME}"
echo

exec fly deploy --app "${APP}" \
  --build-arg "GIT_SHA=${GIT_SHA}" \
  --build-arg "BUILD_TIME=${BUILD_TIME}" \
  "$@"
