#!/bin/sh
# Team profile acceptance (REFRACT_PROFILE=team) on the Docker image:
#   1. it refuses to start without sign-in, and without working isolation;
#   2. with the offline bundle and the sandbox, it compiles a document;
#   3. a document of one account can't see the database or another account's
#      project, while the same probe does see them without the profile (so the
#      probe itself works).
# The assistant's compiles take the same queue and sandbox; the Go tests
# cover them (TestSandboxAssistantSeesOnlyItsWorktree).
#
# Usage: scripts/smoke-team.sh <image> [tex-volume]
# The TeX bundle (about 3 GB) is prepared in the volume once; pass the same
# volume again to reuse it. Needs docker, curl and openssl.
set -eu
IMAGE=$1
TEX=${2:-refract-smoke-tex}
ID=refract-team-$$
PORT=18090
KEY=$(openssl rand -base64 32)
JAR=$(mktemp -d)
# bubblewrap needs user namespaces and its own /proc inside the container.
SANDBOX_OPTS="--security-opt seccomp=unconfined --security-opt apparmor=unconfined --security-opt systempaths=unconfined"
cleanup() {
	docker rm -f "$ID" >/dev/null 2>&1 || true
	docker volume rm "$ID" >/dev/null 2>&1 || true
	rm -rf "$JAR"
}
trap cleanup EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

# refuses <grep pattern> <docker run args…>: the server must exit, saying why.
refuses() {
	want=$1; shift
	if out=$(docker run --rm -e REFRACT_SECRET_KEY="$KEY" "$@" "$IMAGE" 2>&1); then
		fail "started, expected a refusal matching \"$want\""
	fi
	echo "$out" | grep -q "$want" || fail "refused, but without \"$want\":
$out"
	echo "refused: $(echo "$out" | grep "$want" | head -1 | cut -c1-160)"
}

echo "== the team profile refuses what wouldn't isolate accounts"
refuses "needs people to sign in" -e REFRACT_PROFILE=team -e REFRACT_AUTH=none
refuses "Remove REFRACT_SANDBOX=none" -e REFRACT_PROFILE=team -e REFRACT_AUTH=builtin -e REFRACT_SANDBOX=none
refuses "can't be isolated" -e REFRACT_PROFILE=team -e REFRACT_AUTH=builtin
refuses "can't be isolated" -e REFRACT_PROFILE=team -e REFRACT_AUTH=builtin \
	-e REFRACT_TECTONIC_BUNDLE=/opt/refract/tex/bundle -e TECTONIC_CACHE_DIR=/opt/refract/tex/cache

echo "== offline TeX bundle in volume $TEX"
if docker run --rm -v "$TEX:/opt/refract/tex" --entrypoint sh "$IMAGE" -c 'test -d /opt/refract/tex/bundle && test -d /opt/refract/tex/cache'; then
	echo "already prepared"
else
	docker run --rm -v "$TEX:/opt/refract/tex" --entrypoint refract-prepare-bundle "$IMAGE" /opt/refract/tex
fi

# start <extra docker run args…>: a server on $PORT with two accounts, a and b.
start() {
	docker rm -f "$ID" >/dev/null 2>&1 || true
	docker run -d --name "$ID" -p "127.0.0.1:$PORT:8080" -v "$ID:/data" -v "$TEX:/opt/refract/tex:ro" \
		-e REFRACT_SECRET_KEY="$KEY" -e REFRACT_AUTH=builtin \
		-e REFRACT_TECTONIC_BUNDLE=/opt/refract/tex/bundle -e TECTONIC_CACHE_DIR=/opt/refract/tex/cache \
		"$@" "$IMAGE" >/dev/null
	if ! curl -fsS --retry 30 --retry-all-errors --retry-delay 1 "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1; then
		docker logs "$ID" >&2
		fail "the server didn't start"
	fi
	for who in a b; do
		pw=$(docker exec "$ID" refract users add "$who@example.test" | sed -n 's/^  //p' | head -1)
		curl -fsS -c "$JAR/$who" -H 'X-Refract: 1' -H 'Content-Type: application/json' \
			-d "{\"email\":\"$who@example.test\",\"password\":\"$pw\"}" "http://127.0.0.1:$PORT/api/auth/login" >/dev/null ||
			fail "$who can't sign in"
	done
}

# api <account> <method> <path> [body]
api() {
	curl -fsS -b "$JAR/$1" -X "$2" -H 'X-Refract: 1' -H 'Content-Type: application/json' ${4:+--data-binary "$4"} "http://127.0.0.1:$PORT$3"
}

# project <account> <name> <main.tex>: prints the new project's id.
project() {
	id=$(api "$1" POST /api/projects "{\"name\":\"$2\"}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
	[ -n "$id" ] || fail "no project for $1"
	printf '%s' "$3" | api "$1" PUT "/api/projects/$id/file?path=main.tex" @- >/dev/null
	api "$1" PATCH "/api/projects/$id" '{"main_file":"main.tex"}' >/dev/null
	echo "$id"
}

# probe: prints what account a's document could see, after b saved a secret.
probe() {
	theirs=$(project b theirs '\documentclass{article}\begin{document}secret of b\end{document}')
	doc="\\documentclass{article}
\\begin{document}
\\IfFileExists{/data/refract.db}{\\typeout{PROBE database visible}}{\\typeout{PROBE database hidden}}
\\IfFileExists{/data/projects/$theirs/main.tex}{\\typeout{PROBE other-project visible}}{\\typeout{PROBE other-project hidden}}
\\IfFileExists{main.tex}{\\typeout{PROBE own-project visible}}{\\typeout{PROBE own-project hidden}}
Hello from a.
\\end{document}"
	mine=$(project a mine "$doc")
	res=$(api a POST "/api/projects/$mine/compile")
	echo "$res" | grep -q '"ok":true' || fail "a's compile failed: $res"
	api a GET "/api/projects/$mine/pdf" | head -c 5 | grep -q '%PDF' || fail "no PDF"
	# b's project is not a's: the API refuses it too.
	if api a GET "/api/projects/$theirs/file?path=main.tex" >/dev/null 2>&1; then fail "a read b's file through the API"; fi
	api a GET "/api/projects/$mine/log" | sed -n 's/^PROBE //p'
}

echo "== control: without the profile, the probe sees everything (it works)"
start
seen=$(probe)
echo "$seen"
echo "$seen" | grep -q "database visible" || fail "the probe can't see the database even without isolation: it proves nothing"
echo "$seen" | grep -q "other-project visible" || fail "the probe can't see b's project even without isolation: it proves nothing"
docker logs "$ID" 2>&1 | grep -q "compiles are not isolated" || fail "no warning about unisolated compiles"
echo "warned at startup: compiles are not isolated"
docker rm -f "$ID" >/dev/null
docker volume rm "$ID" >/dev/null

echo "== team profile: compiles, and keeps accounts apart"
start -e REFRACT_PROFILE=team $SANDBOX_OPTS
seen=$(probe)
echo "$seen"
echo "$seen" | grep -q "own-project visible" || fail "a's document can't see its own project"
echo "$seen" | grep -q "database hidden" || fail "a's document can see the database"
echo "$seen" | grep -q "other-project hidden" || fail "a's document can see b's project"
docker logs "$ID" 2>&1 | grep -q '"sandbox":"bwrap"' || fail "the server didn't report the sandbox"
echo "== all ok"
