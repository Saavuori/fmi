#!/usr/bin/env bash
#
# install / update the Tutka stack.
#
# Idempotent: run it for a first install or to apply an update. It writes
# docker-compose.yml and update.sh, ensures the shared proxy network and the
# frame-archive volume exist, registers the auto-update cron (every 5 min),
# pulls the latest image, recreates the containers, and then verifies that the
# radar archive actually came up — a compose missing the /data volume leaves the
# app serving only in-memory frames, which looks fine until a restart.
#
# TLS is expected to be terminated by an external reverse proxy (e.g. Caddy)
# attached to the shared external `web-proxy` network — this stack only exposes
# the backend on that network, not on a host port.
#
# Usage:
#   ./install.sh                        # default domain (tutka.duckdns.org)
#   ./install.sh saa.example.org        # custom domain as an argument
#   DOMAIN=saa.example.org ./install.sh              # ...or via env
#   APP_DIR=/srv/tutka IMAGE=ghcr.io/you/fmi:latest ./install.sh
#
# The domain is only used for the post-deploy health checks and must resolve to
# the reverse proxy in front of this stack — remember to add the matching vhost
# to your Caddy/nginx config on the `web-proxy` network AND reload the proxy
# (Caddy only picks up a Caddyfile edit on `caddy reload`).
#
set -euo pipefail

# --- config (first argument or env, with defaults) --------------------------
APP_DIR="${APP_DIR:-$HOME/tutka}"
DOMAIN="${1:-${DOMAIN:-tutka.duckdns.org}}"   # used for the post-deploy checks
IMAGE="${IMAGE:-ghcr.io/saavuori/fmi:latest}"

# --- pick a container engine + compose command ------------------------------
if command -v podman-compose >/dev/null 2>&1; then
  ENGINE="podman"; COMPOSE="podman-compose"
elif command -v podman >/dev/null 2>&1 && podman compose version >/dev/null 2>&1; then
  ENGINE="podman"; COMPOSE="podman compose"
elif command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  ENGINE="docker"; COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  ENGINE="docker"; COMPOSE="docker-compose"
else
  echo "ERROR: need podman-compose, 'podman compose', or Docker Compose installed." >&2
  exit 1
fi
echo "engine=$ENGINE  compose='$COMPOSE'  app_dir=$APP_DIR  domain=$DOMAIN"

# --- write the compose file (backing up any existing one) -------------------
mkdir -p "$APP_DIR"
cd "$APP_DIR"
if [ -f docker-compose.yml ]; then
  backup="docker-compose.yml.bak-$(date +%s)"
  cp docker-compose.yml "$backup"
  echo "backed up existing compose -> $APP_DIR/$backup"
fi

cat > docker-compose.yml <<'YAML'
# Managed by install.sh — edit there, not here.
services:
  fmi-backend:
    image: ghcr.io/saavuori/fmi:latest
    container_name: fmi-backend
    restart: unless-stopped
    environment:
      - REDIS_URL=redis://fmi-cache:6379
      - PORT=8080
      # Radar frame archive. Frames are files, not database rows: pruning them
      # must not need scratch space, because this host's root volume runs close
      # to full.
      - FRAMES_DIR=/data/frames
      # Per-product retention in hours. dbz matches the 7 days FMI itself keeps;
      # rr is held for 2 days because it is a 16-bit product and so roughly twice
      # the bytes per frame. Budget at the default grid is ~250-450 MB.
      - TUTKA_RETENTION_HOURS=dbz=168,rr=48,rr1h=168,rr12h=168,rr24h=168
      # The canonical grid: EPSG:3857 bbox and mercator metres per pixel.
      - GRID_BBOX=2000000,8250000,3600000,11200000
      - GRID_RESOLUTION=2000
    volumes:
      # Persist the frame archive across container recreation. `:Z` relabels the
      # volume for SELinux (Oracle Linux enforces it). Without this mount the
      # archive directory can't be created and only in-memory frames are served.
      - fmi-frames:/data:Z
    networks:
      - default
      - web-proxy
    depends_on:
      - fmi-cache
  fmi-cache:
    image: docker.io/library/redis:8-alpine
    container_name: fmi-cache
    restart: unless-stopped
    command: redis-server --appendonly no --maxmemory 64mb --maxmemory-policy allkeys-lru
    networks:
      - default
volumes:
  # Named volume for the radar frame archive (survives container recreation).
  fmi-frames:
networks:
  default:
  web-proxy:
    external: true
YAML
echo "wrote $APP_DIR/docker-compose.yml"

# --- write the auto-update script + register its cron -----------------------
cat > update.sh <<'SH'
#!/usr/bin/env bash
# Auto-update script for the Tutka stack. Managed by install.sh, which
# registers a cron entry that runs it every 5 minutes. It checks ghcr.io for a
# new image and redeploys when one appears.
# (Watchtower is not used — it is incompatible with rootless Podman on RHEL.)
set -euo pipefail

COMPOSE_DIR="$(cd "$(dirname "$0")" && pwd)"
LOG="$COMPOSE_DIR/update.log"
IMAGE="${IMAGE:-ghcr.io/saavuori/fmi:latest}"

if command -v podman-compose >/dev/null 2>&1; then
  ENGINE="podman"; COMPOSE="podman-compose"
elif command -v podman >/dev/null 2>&1 && podman compose version >/dev/null 2>&1; then
  ENGINE="podman"; COMPOSE="podman compose"
elif command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  ENGINE="docker"; COMPOSE="docker compose"
else
  ENGINE="docker"; COMPOSE="docker-compose"
fi

echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Checking for updates..." >> "$LOG"

$ENGINE pull "$IMAGE" >> "$LOG" 2>&1

NEW_ID=$($ENGINE inspect "$IMAGE" --format '{{.Id}}')
RUNNING_ID=$($ENGINE inspect fmi-backend --format '{{.Image}}' 2>/dev/null || echo '')

if [ "$RUNNING_ID" = "$NEW_ID" ]; then
  echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Already up to date." >> "$LOG"
  exit 0
fi

echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] New image detected! Redeploying..." >> "$LOG"

# Full down/up — the only reliable way with rootless Podman
cd "$COMPOSE_DIR"
$COMPOSE down >> "$LOG" 2>&1 || true
$COMPOSE up -d >> "$LOG" 2>&1

echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] Redeploy complete." >> "$LOG"
SH
chmod +x update.sh
echo "wrote $APP_DIR/update.sh"

if crontab -l 2>/dev/null | grep -qF "$APP_DIR/update.sh"; then
  echo "auto-update cron already registered"
else
  ( crontab -l 2>/dev/null || true; echo "*/5 * * * * $APP_DIR/update.sh" ) | crontab -
  echo "registered auto-update cron: */5 * * * * $APP_DIR/update.sh"
fi

# --- ensure the shared external proxy network exists ------------------------
if ! $ENGINE network exists web-proxy >/dev/null 2>&1; then
  echo "creating shared 'web-proxy' network..."
  $ENGINE network create web-proxy
fi

# --- pull + recreate --------------------------------------------------------
echo "pulling $IMAGE ..."
$ENGINE pull "$IMAGE" || echo "(pull failed or offline; using local image)"

echo "recreating stack..."
# down first so the new env + volume are guaranteed to apply; the external
# web-proxy network is not removed by down.
$COMPOSE down --remove-orphans >/dev/null 2>&1 || true
$COMPOSE up -d

# --- verify the radar archive ----------------------------------------------
# The first frame is fetched within ~60s of boot; the backfill then walks back
# through the retention window, so frames_on_disk keeps climbing after this.
echo "verifying the radar archive (first frame lands within ~60s)..."
ok=0
for i in $(seq 1 12); do
  sleep 10
  health="$(curl -fsS "https://$DOMAIN/api/health" 2>/dev/null || true)"
  case "$health" in
    *'"archive_enabled":true'*)
      case "$health" in
        *'"frames_on_disk":0'*) ;;   # mounted but nothing fetched yet
        *) ok=1; break ;;
      esac
      ;;
  esac
  echo "  ...no frames yet (check $i/12)"
done

echo
echo "=== /api/health ==="
curl -fsS "https://$DOMAIN/api/health" 2>/dev/null || echo "(health unreachable from here)"
echo
if [ "$ok" = "1" ]; then
  echo "OK: the radar archive is recording."
  echo "   The animation needs >=2 frames, so the loop appears after ~5-10 min;"
  echo "   the backfill keeps filling the window for a while after that."
  echo "   Watch disk use as it fills:  $ENGINE exec fmi-backend du -sh /data/frames"
else
  echo "NOT CONFIRMED. Inspect the backend logs:"
  echo "   $ENGINE logs fmi-backend 2>&1 | grep -iE 'tutka|frame|archive'"
  echo "   ('archive disabled' means the /data volume still isn't writable)"
fi
