#!/usr/bin/env python3
"""Disposable real-image test: isolated network, test wallet, no blockchain writes."""
import base64
import tempfile
import importlib.util
import json
import pathlib
import secrets
import subprocess
import sys
import time
import uuid

root = pathlib.Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("verify_demo", root / "scripts/deploy/verify-demo.py")
checks = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checks)
image, version = sys.argv[1:]
name = "wallet-smoke-" + uuid.uuid4().hex[:12]
volume = name + "-data"
probe_dir = tempfile.TemporaryDirectory(prefix="wallet-probe-")


def docker(*args):
    return subprocess.check_output(["docker", *args], text=True).strip()


def ready():
    for _ in range(45):
        if docker("inspect", "--format", "{{.State.Health.Status}}", name) == "healthy":
            return
        if docker("inspect", "--format", "{{.State.Running}}", name) != "true":
            raise AssertionError("container exited")
        time.sleep(1)
    raise AssertionError("container did not become healthy")



def isolated_request(port, path, method="GET", headers=None, body=None):
    response = json.loads(subprocess.check_output(
        ["docker", "exec", "-i", name, "/probe"],
        input=json.dumps({"Path": path, "Method": method, "Headers": headers or {}, "Body": body or ""}), text=True))
    return response["Status"], response["Headers"], base64.b64decode(response["Body"])

checks.request = isolated_request

try:
    arch = docker("image", "inspect", "--format", "{{.Architecture}}", image)
    docker("run", "--rm", "--network", "none", "-e", "GOARCH="+arch, "-v", str(root)+":/src:ro", "-v", probe_dir.name+":/out", "-e", "GOCACHE=/tmp/cache", "-e", "GO111MODULE=off", "-e", "CGO_ENABLED=0", "golang:1.26-alpine@sha256:ce864e7223ac17b1775e6fd0b4c0db580c2eb50e7953a427916379e4b92a1628", "go", "build", "-o", "/out/probe", "/src/tests/deployment/probe.go")
    docker("volume", "create", volume)
    # Reuse the already available build image only to set disposable volume ownership.
    docker("run", "--rm", "--network", "none", "-v", volume+":/data", "golang:1.26-alpine@sha256:ce864e7223ac17b1775e6fd0b4c0db580c2eb50e7953a427916379e4b92a1628", "chown", "10001:10001", "/data")
    args = ["run", "--platform", "linux/"+arch, "-d", "--name", name, "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--memory", "640m", "--memory-swap", "640m", "-e", "GOMEMLIMIT=480MiB", "--cpus", "1", "--pids-limit", "128", "-v", probe_dir.name+"/probe:/probe:ro", "-v", volume+":/data/wallet", "-e", "PUBLIC_ORIGIN=https://wallet.example", "-e", "WALLET_ACCESS_TOKEN="+secrets.token_hex(32)]
    for key in ["SEPOLIA", "ARBITRUM_SEPOLIA", "BASE_SEPOLIA", "OP_SEPOLIA", "POLYGON_AMOY", "SOLANA_DEVNET", "TRON_SHASTA"]:
        args.extend(["-e", key+"_RPC_URL=http://127.0.0.1:1"])
    docker(*args, image)
    ready()
    port, headers = checks.verify(name, version, port=8090)
    status, _, data = checks.request(port, "/api/wallet", headers=headers)
    before = json.loads(data)
    assert status == 200 and not before["exists"]
    headers.update({"X-Wallet-CSRF": before["csrfToken"], "Content-Type": "application/json"})
    wallet_password = secrets.token_hex(20)
    status, _, data = checks.request(port, "/api/wallet/create", "POST", headers, json.dumps({"password": wallet_password}))
    assert status == 200, "test wallet creation failed"
    # Never print the create response: it contains the disposable recovery phrase.
    status, _, data = checks.request(port, "/api/wallet", headers=headers)
    address = json.loads(data)["address"]
    stale_cookie = headers["Cookie"]
    docker("stop", "-t", "45", name)
    docker("rm", name)
    docker(*args, image)
    ready()
    port, headers = checks.verify(name, version, port=8090)
    assert checks.request(port, "/api/wallet", headers={**headers, "Cookie": stale_cookie})[0] == 401
    status, _, data = checks.request(port, "/api/wallet", headers=headers)
    assert status == 200 and json.loads(data)["address"] == address, "keystore lost during replacement"
    assert docker("inspect", "--format", "{{.Config.User}}", name) == "10001:10001"
    docker("stop", "-t", "45", name)
    docker("rm", name)
    docker(*args, "-e", "SHARED_DEMO=true", image)
    ready()
    port, shared_headers = checks.verify(name, version, port=8090)
    status, _, data = checks.request(port, "/api/wallet", headers=shared_headers)
    shared = json.loads(data)
    assert shared["address"] == address, "shared mode changed wallet"
    shared_headers.update({"X-Wallet-CSRF": shared["csrfToken"], "Content-Type": "application/json"})
    for password, expected in [("incorrect-password", 401), (wallet_password, 200)]:
        status, _, _ = checks.request(port, "/api/wallet/backup", "POST", shared_headers, json.dumps({"password": password}))
        assert status == expected, "shared export password check failed"
    for path in ["/api/wallet/create", "/api/wallet/accounts/update", "/api/wallet/scan", "/api/wallet/password"]:
        assert checks.request(port, path, "POST", shared_headers, "{}")[0] == 403, "CSRF must not authorize management"
    print("PASS: shared mode uses same wallet without login; password and management restrictions verified")
    print("PASS: real image, non-root/read-only runtime, login/CSRF, container replacement persistence, stale session rejection; no live RPC")
finally:
    for args in [("rm", "-f", name), ("volume", "rm", volume)]:
        subprocess.run(["docker", *args], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    probe_dir.cleanup()
