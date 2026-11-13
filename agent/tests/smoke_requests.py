"""Live local stack test: creates labelled test requests; does not delete user data.
Run from agent: PYTHONPATH=src .venv/bin/python tests/smoke_requests.py
"""
import json
import urllib.request
import urllib.error
from datetime import date, timedelta
from uuid import uuid4
from concurrent.futures import ThreadPoolExecutor
from clients.mcp import MCPGateway
from types import SimpleNamespace

def request(path, body=None, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request("http://localhost:8080" + path,
        data=None if body is None else json.dumps(body).encode(), headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode()

def main():
    a = request("/api/login", {"login": "sasha.intern"})[1]["token"]
    b = request("/api/login", {"login": "maria.intern"})[1]["token"]
    assert request("/api/requests")[0] == 401
    assert request("/api/request-types", token=a)[1][0]["code"] == "time_off"
    # Different future dates on each run, with an explicit diagnostic reason.
    d = date.today() + timedelta(days=365 + int(uuid4().hex[:4], 16) % 365)
    body = {"start_date": str(d), "end_date": str(d), "reason": "SMOKE TEST",
            "idempotency_key": str(uuid4())}
    with ThreadPoolExecutor(max_workers=2) as pool:
        replies = list(pool.map(lambda _: request("/api/requests/time-off", body, a), range(2)))
    assert all(x[0] == 200 for x in replies), replies
    row = replies[0][1]
    assert row["id"] == replies[1][1]["id"]
    assert row["status"] == "submitted"
    assert request("/api/requests/time-off", {**body, "reason": "changed"}, a)[0] == 409
    assert request("/api/requests/time-off", {**body, "idempotency_key": str(uuid4())}, a)[0] == 409
    assert request("/api/requests/time-off", {**body, "start_date": "2000-01-01", "idempotency_key": str(uuid4())}, a)[0] == 422
    assert all(x["id"] != row["id"] for x in request("/api/requests", token=b)[1])
    assert request("/api/requests/time-off", {**body, "user_id": 999}, a)[0] == 400
    gateway = MCPGateway(SimpleNamespace(mcp_url="http://localhost:8090/mcp", timeout=30))
    types = gateway.call("get_request_types", {}, a)
    assert "time_off" in json.dumps(types)
    listed = gateway.call("list_my_requests", {}, a)
    assert str(row["id"]) in json.dumps(listed)
    replay = gateway.call("create_time_off_request", body, a)
    assert replay["id"] == row["id"], replay
    invalid = gateway.call("create_time_off_request", {**body, "end_date": "bad", "idempotency_key": str(uuid4())}, a)
    assert "error" in invalid, invalid
    print("PASS: auth, templates, concurrent replay, conflict, dates, ownership, MCP create/list/errors")

if __name__ == "__main__":
    main()
