#!/usr/bin/env bash
# Releases of bm-open on the server. Run from /opt/bm-open (or anywhere: it
# moves to the repo root itself).
#
#   deploy/release.sh                  git pull, build, switch, restart, check
#   deploy/release.sh install <file>   same with a binary built elsewhere (make build-linux)
#   deploy/release.sh rollback         back to the previous release
#
# Every release lives in releases/<UTC time>/bm-open; live points at the
# current one. If the new release does not answer /healthz, live goes back
# to the old one and the script fails.
set -euo pipefail

cd "$(dirname "$0")/.."

APP=bm-open
KEEP=5

addr=127.0.0.1:3310
if [[ -f .env ]]; then
	from_env=$(sed -n 's/^LISTEN_ADDR=//p' .env | tail -n 1 | tr -d '"'"'"' ')
	addr=${from_env:-$addr}
fi

log() { printf '%s\n' "$*" >&2; }
die() { log "error: $*"; exit 1; }

current() { [[ -L live ]] && readlink live || true; }

switch_to() {
	ln -sfn "$1" live
	if pm2 describe "$APP" >/dev/null 2>&1; then
		pm2 restart "$APP" --update-env >/dev/null
	else
		# First run on this server.
		pm2 start deploy/ecosystem.config.cjs >/dev/null
		pm2 save >/dev/null
	fi
}

# healthy waits for /healthz to report the version in $1, so an old process
# still holding the port does not pass for the new one.
healthy() {
	local i answer
	for i in $(seq 1 15); do
		answer=$(curl -fsS --max-time 2 "http://$addr/healthz" 2>/dev/null || true)
		if [[ $answer == "ok $1" ]]; then
			return 0
		fi
		sleep 1
	done
	log "healthz answered: ${answer:-nothing}"
	return 1
}

# activate switches to the release in $1 and returns to $2 if it fails.
activate() {
	local next=$1 prev=$2 version
	version=$("$next/$APP" -version) || die "$next/$APP does not start"
	log "switching live: ${prev:-none} -> $next ($version)"
	switch_to "$next"
	if healthy "$version"; then
		log "ok: $next is live"
		return 0
	fi
	if [[ -n $prev ]]; then
		log "no answer from http://$addr/healthz, back to $prev"
		switch_to "$prev"
		healthy "$("$prev/$APP" -version)" || log "the previous release does not answer either: check pm2 logs $APP"
	fi
	die "$next did not start; see pm2 logs $APP"
}

prune() {
	local keep_now old
	keep_now=$(current)
	# Names are UTC timestamps, so a plain sort is by age.
	ls -1d releases/*/ 2>/dev/null | sed 's:/$::' | sort -r | tail -n +$((KEEP + 1)) | while read -r old; do
		[[ $old == "$keep_now" ]] && continue
		rm -rf -- "$old"
		log "removed $old"
	done
}

new_dir() {
	local dir
	dir=releases/$(date -u +%Y%m%dT%H%M%SZ)
	[[ -e $dir ]] && die "$dir already exists, try again in a second"
	mkdir -p "$dir"
	printf '%s' "$dir"
}

[[ -f .env ]] || die ".env is missing: copy .env.example, fill it in, chmod 600 .env"
mkdir -p releases data

case ${1:-release} in
release)
	git pull --ff-only
	command -v go >/dev/null || die "go is not installed (README: «Go на сервере»), or use: $0 install <binary>"
	dir=$(new_dir)
	version=$(git describe --always --dirty 2>/dev/null || echo unknown)
	log "building $version into $dir"
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$dir/$APP" ./cmd/bm-open ||
		{ rm -rf -- "$dir"; die "build failed"; }
	activate "$dir" "$(current)"
	prune
	;;
install)
	src=${2:-}
	[[ -f $src ]] || die "usage: $0 install <path to bm-open built by make build-linux>"
	dir=$(new_dir)
	install -m 0755 "$src" "$dir/$APP"
	"$dir/$APP" -version >/dev/null 2>&1 || { rm -rf -- "$dir"; die "$src does not run here (wrong GOARCH?)"; }
	activate "$dir" "$(current)"
	prune
	;;
rollback)
	now=$(current)
	[[ -n $now ]] || die "live is not set: nothing to roll back"
	prev=$(ls -1d releases/*/ 2>/dev/null | sed 's:/$::' | sort | awk -v now="$now" '$0 < now' | tail -n 1)
	[[ -n $prev ]] || die "no release older than $now"
	activate "$prev" "$now"
	;;
*)
	die "usage: $0 [release | install <binary> | rollback]"
	;;
esac
