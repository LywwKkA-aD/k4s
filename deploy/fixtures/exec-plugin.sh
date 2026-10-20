#!/bin/sh
# A client-go exec credential plugin for the dev cluster that behaves like the
# slow cloud ones (aws eks get-token, gke-gcloud-auth-plugin, kubelogin): it
# writes progress to stderr, takes a few seconds, then prints an
# ExecCredential carrying a ServiceAccount token read from a file.
#
# --require-tty refuses to run unless stdin is a terminal, like a plugin that
# has to prompt for a login. Pair it with interactiveMode: IfAvailable so
# client-go only passes stdin through when it has a terminal.
#
# usage: exec-plugin.sh --token-file <path> [--delay <seconds>] [--require-tty]
# K4S_PLUGIN_DELAY overrides the default 3 second delay.
set -eu

token_file=
delay=${K4S_PLUGIN_DELAY:-3}
require_tty=0

while [ $# -gt 0 ]; do
  case $1 in
    --token-file)
      token_file=${2:?--token-file needs a path}
      shift 2
      ;;
    --delay)
      delay=${2:?--delay needs seconds}
      shift 2
      ;;
    --require-tty)
      require_tty=1
      shift
      ;;
    *)
      echo "k4s exec-plugin: unknown argument $1" >&2
      exit 2
      ;;
  esac
done

if [ -z "$token_file" ]; then
  echo "k4s exec-plugin: --token-file is required" >&2
  exit 2
fi
if [ "$require_tty" = 1 ] && [ ! -t 0 ]; then
  echo "k4s exec-plugin: stdin is not a terminal, refusing to prompt for a login" >&2
  exit 1
fi
if [ ! -r "$token_file" ]; then
  echo "k4s exec-plugin: cannot read $token_file; run make kubeconfig-plugin" >&2
  exit 1
fi

echo "k4s exec-plugin: fetching a token for k4s-viewer (${delay}s)..." >&2
sleep "$delay"

token=$(tr -d '[:space:]' <"$token_file")
# A ServiceAccount token is a JWT (base64url segments joined by dots); anything
# else would break the JSON below.
case $token in
  '' | *[!A-Za-z0-9._-]*)
    echo "k4s exec-plugin: $token_file does not hold a JWT" >&2
    exit 1
    ;;
esac

printf '{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{"token":"%s"}}\n' "$token"
