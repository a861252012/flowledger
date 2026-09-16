#!/usr/bin/env python3
"""Read-only origin smoke checks. Never prints credentials or wallet responses."""
import http.client
import json
import subprocess
import sys
import urllib.parse


def request(port, path, method="GET", headers=None, body=None):
    connection = http.client.HTTPConnection("127.0.0.1", port, timeout=40)
    try:
        connection.request(method, path, body=body, headers=headers or {})
        response = connection.getresponse()
        return response.status, dict(response.getheaders()), response.read()
    finally:
        connection.close()


def verify(container, version, port=None):
    info = json.loads(subprocess.check_output(["docker", "inspect", container]))[0]
    env = dict(item.split("=", 1) for item in info["Config"]["Env"])
    origin, token = env["PUBLIC_ORIGIN"], env["WALLET_ACCESS_TOKEN"]
    if port is None:
        port = int(info["NetworkSettings"]["Ports"]["8090/tcp"][0]["HostPort"])
    actual = subprocess.check_output(["docker", "exec", container, "/wallet", "--version"], text=True).strip()
    assert actual == version, "running binary version differs"
    headers = {"Host": urllib.parse.urlsplit(origin).netloc, "Origin": origin}
    status, _, page = request(port, "/", headers={**headers, "Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"})
    assert status == 200 and b'public-query' in page, "public demo page missing"
    for path in ("/api/wallet/accounts", "/api/wallet/history", "/solana/api/status", "/tron/api/status", "/api/faucet"):
        assert request(port, path, headers=headers)[0] == 401, "private route exposed"
    assert request(port, "/api/wallet", headers=headers)[0] == 401, "anonymous API accepted"
    assert request(port, "/api/wallet", headers={**headers, "Host": "evil.example"})[0] == 403, "wrong Host accepted"
    status, response_headers, _ = request(port, "/login", "POST", {
        **headers, "Content-Type": "application/x-www-form-urlencoded",
        "X-Forwarded-Proto": "http",
    }, urllib.parse.urlencode({"token": token}))
    assert status == 303, "public origin login failed"
    cookie = response_headers.get("Set-Cookie", "")
    assert all(item in cookie for item in ("Secure", "HttpOnly", "SameSite=Strict")), "cookie flags missing"
    authenticated = {**headers, "Cookie": cookie.split(";", 1)[0]}
    assert request(port, "/api/wallet", headers=authenticated)[0] == 200, "authenticated wallet rejected"
    assert request(port, "/api/wallet", headers={**authenticated, "Origin": "https://evil.example"})[0] == 403, "wrong Origin accepted"
    assert request(port, "/api/wallet/create", "POST", {**authenticated, "Content-Type": "application/json"}, "{}")[0] == 403, "missing CSRF accepted"
    return port, authenticated


if __name__ == "__main__":
    verify(*sys.argv[1:])
    print("PASS: version, authentication, public origin, cookie and CSRF checks")
