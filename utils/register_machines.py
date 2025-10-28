#!/usr/bin/env python3
"""
Register machines with the API (new endpoints).

Usage:
  python utils/register_machines.py provision/machines.yml

YAML format:
machines:
  - name: alpine-1
    host: localhost
    port: 22221
    user: root
    password: test

Environment variables:
  API_BASE: Base URL for the site (default: https://localhost)
  API_PREFIX: Path prefix to the API (default: /api)
  ADMIN_DEFAULT_USERNAME: Admin username for login (default: admin)
  ADMIN_DEFAULT_PASSWORD: Admin password for login (default: change-me)
  VERIFY_TLS: "true"/"false" to verify TLS (default: false for local self-signed)
  REWRITE_LOCALHOST: "true"/"false" rewrite localhost → host.docker.internal (default: true)
  DOCKER_HOST_GATEWAY_NAME: name to use instead of localhost (default: host.docker.internal)
"""
from __future__ import annotations
import json
import os
import sys
from typing import Any, Dict, List

import requests

try:
    import yaml  # PyYAML
except Exception:
    yaml = None  # We'll error nicely below.

import urllib3
urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)


def env_flag(name: str, default: bool) -> bool:
    v = os.getenv(name)
    if v is None:
        return default
    return str(v).strip().lower() in ("1", "true", "yes", "on")


API_BASE = os.getenv("API_BASE", "https://localhost").rstrip("/")
API_PREFIX = os.getenv("API_PREFIX", "/api")
API_PREFIX = ("" if not API_PREFIX else ("/" + API_PREFIX.lstrip("/")))

ADMIN_USER = os.getenv("ADMIN_DEFAULT_USERNAME", "admin")
ADMIN_PASS = os.getenv("ADMIN_DEFAULT_PASSWORD", "change-me")
VERIFY_TLS = env_flag("VERIFY_TLS", False)

REWRITE_LOCALHOST = env_flag("REWRITE_LOCALHOST", True)
DOCKER_HOST_GATEWAY_NAME = os.getenv("DOCKER_HOST_GATEWAY_NAME", "host.docker.internal")


def api_url(path: str) -> str:
    # Join base + prefix + path, ensuring single slashes
    base = API_BASE
    prefix = API_PREFIX
    if not path.startswith("/"):
        path = "/" + path
    return f"{base}{prefix}{path}"


def parse_machines_file(path: str) -> List[Dict[str, Any]]:
    if yaml is None:
        raise RuntimeError("PyYAML is not installed. Run: pip install pyyaml")

    with open(path, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f)

    if data is None:
        return []

    # Support either a top-level list or {"machines": [...]}
    items = data.get("machines") if isinstance(data, dict) else data
    if not isinstance(items, list):
        raise ValueError("YAML must be a list of machines or contain a top-level 'machines' list.")

    out: List[Dict[str, Any]] = []
    for m in items:
        if not isinstance(m, dict):
            continue
        name = str(m.get("name", "")).strip()
        host = str(m.get("host", "")).strip()
        port = m.get("port", None)
        user = str(m.get("user", "")).strip()
        password = str(m.get("password", "")).strip()
        if not all([name, host, port, user, password]):
            print(f"Skipping incomplete entry: {m}")
            continue
        try:
            port = int(port)
        except Exception:
            print(f"Skipping entry with invalid port: {m}")
            continue
        out.append({"name": name, "host": host, "port": port, "user": user, "password": password})
    return out


def login_for_token(user: str, password: str) -> str:
    # New API endpoint
    url = api_url("/auth/login")
    try:
        resp = requests.post(url, json={"username": user, "password": password}, verify=VERIFY_TLS)
    except requests.RequestException as e:
        print(f"Login request failed: {e}")
        return ""
    if resp.status_code != 200:
        print(f"Login failed ({resp.status_code}): {resp.text}")
        return ""
    try:
        data = resp.json()
    except ValueError:
        print("Login response was not JSON")
        return ""
    token = data.get("access_token") or ""
    if not token:
        print("No access_token in login response")
    return token


def post_machine(token: str, m: Dict[str, Any]) -> requests.Response:
    url = api_url("/machines")
    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}
    payload = dict(m)

    if REWRITE_LOCALHOST and payload.get("host") in ("localhost", "127.0.0.1"):
        payload["host"] = DOCKER_HOST_GATEWAY_NAME

    return requests.post(url, headers=headers, data=json.dumps(payload), verify=VERIFY_TLS)


def main(argv: List[str]) -> int:
    if len(argv) != 2:
        print("Usage: python utils/register_machines.py <machines.yml>")
        return 1

    machines_file = argv[1]
    if not os.path.exists(machines_file):
        print(f"File not found: {machines_file}")
        return 1

    try:
        machines = parse_machines_file(machines_file)
    except Exception as e:
        print(f"Failed to parse {machines_file}: {e}")
        return 1

    if not machines:
        print("No machines to register.")
        return 0

    token = login_for_token(ADMIN_USER, ADMIN_PASS)
    if not token:
        print("Cannot continue without admin token. Check credentials and API endpoint.")
        print(f"API_BASE={API_BASE} API_PREFIX={API_PREFIX} VERIFY_TLS={VERIFY_TLS}")
        return 2

    any_failed = False
    for m in machines:
        try:
            resp = post_machine(token, m)
        except requests.RequestException as e:
            any_failed = True
            print(f"Error adding {m.get('name')}: {e}")
            continue

        if resp.status_code in (200, 201):
            try:
                body = resp.json()
            except ValueError:
                body = {}
            host_display = body.get("host", m.get("host"))
            port_display = body.get("port", m.get("port"))
            print(f"Added {m['name']} ({host_display}:{port_display})")
        else:
            any_failed = True
            print(f"Failed to add {m['name']}: {resp.status_code} {resp.text}")

    return 2 if any_failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
