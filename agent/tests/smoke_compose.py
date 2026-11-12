"""Isolated Go + PostgreSQL + MCP + Flask smoke test; deterministic Ollama stub.
Run: python tests/smoke_compose.py (Docker Compose required).
Only the uniquely named test project's containers/volume are removed.
"""
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
MOCK = r'''
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json, time
class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        messages = body.get("messages", [])
        prompt = json.dumps(messages, ensure_ascii=False)
        assert "Саша Иванов" in prompt, "verified whoami profile missing"
        assert not any(t["function"]["name"] == "search_history" for t in body.get("tools", []))
        assert "Bearer" not in prompt
        assert "FAILED-HISTORY" not in prompt
        if messages[-1].get("content") == "second":
            assert any(m.get("content") == "first" for m in messages), "history missing"
        self.send_response(200)
        self.send_header("Content-Type", "application/x-ndjson")
        self.end_headers()
        for text in ["Проверка ", "потока ", "успешна."]:
            item = {"model":body["model"],"message":{"role":"assistant","content":text},"done":False}
            self.wfile.write((json.dumps(item)+"\n").encode()); self.wfile.flush()
            time.sleep(0.15)
        self.wfile.write((json.dumps({"model":body["model"],"message":{"role":"assistant","content":""},"done":True,"done_reason":"stop"})+"\n").encode())
ThreadingHTTPServer(("0.0.0.0",11434),Handler).serve_forever()
'''
CLIENT = r'''
import json, time, os, urllib.request, urllib.error
base="http://api:8080"
def req(path, data=None, token=None, headers=None):
    h={"Content-Type":"application/json", **(headers or {})}
    if token: h["Authorization"]="Bearer "+token
    return urllib.request.urlopen(urllib.request.Request(base+path,
        data=None if data is None else json.dumps(data).encode(),headers=h),timeout=30)
def obj(path,data=None,token=None):
    with req(path,data,token) as response: return json.load(response)
token=obj("/api/login",{"login":"sasha.intern"})["token"]
cid=obj("/api/conversations",{"title":"isolated smoke"},token)["id"]
# A failed run must remain in the database but never enter the LLM context.
headers={"X-Agent-Key":os.environ["AGENT_SERVICE_TOKEN"]}
with req(f"/internal/agent/conversations/{cid}/turns",
         {"client_message_id":"failed-run","payload":{"content":"FAILED-HISTORY"}},token,headers) as response:
    failed=json.load(response)["run_id"]
with req(f"/internal/agent/conversations/{cid}/runs/{failed}/messages",
         {"event_key":"a","role":"assistant","payload":{"content":"FAILED-HISTORY"}},token,headers): pass
with req(f"/internal/agent/conversations/{cid}/runs/{failed}/finish",{"status":"failed"},token,headers): pass
for index, content in enumerate(["first","second"]):
    data={"conversation_id":cid,"client_message_id":"smoke-"+str(index),"content":content}
    # A forged browser service key must be replaced by Go.
    with req("/api/chat",data,token,{"X-Agent-Key":"wrong-browser-key"}) as response:
        names=[]; times=[]; text=""
        for line in response:
            line=line.decode().strip()
            if line.startswith("event: "): names.append(line[7:])
            if line.startswith("data: "):
                event=json.loads(line[6:])
                if names[-1]=="text_delta":
                    times.append(time.monotonic()); text+=event["text"]
                assert names[-1]!="error", event
        assert names[-1]=="done", names
        assert "warning" not in names, names
        assert text=="Проверка потока успешна.", text
        assert len(times)>=3 and times[-1]-times[0]>.2, "SSE was buffered"
    try: req("/api/chat",data,token)
    except urllib.error.HTTPError as exc: assert exc.code==409, exc.code
    else: raise AssertionError("duplicate accepted")
history=obj(f"/api/conversations/{cid}/messages",token=token)["messages"]
assert len(history)==6, history
other=obj("/api/login",{"login":"maria.intern"})["token"]
try: req(f"/api/conversations/{cid}/messages",token=other)
except urllib.error.HTTPError as exc: assert exc.code==404, exc.code
else: raise AssertionError("cross-user access allowed")
try: req("/api/chat",data,"invalid")
except urllib.error.HTTPError as exc: assert exc.code==401, exc.code
else: raise AssertionError("invalid JWT accepted")
print("PASS: real Go auth + MCP whoami + Flask + SSE proxy + PostgreSQL history; duplicate and cross-user checks")
'''
def main():
    project = "buddy-smoke-" + secrets.token_hex(4)
    with tempfile.TemporaryDirectory(prefix="buddy-smoke-") as tmp:
        tmp = Path(tmp)
        env_text = (ROOT / ".env.example").read_text()
        env_text += "\nAGENT_KEY=" + secrets.token_hex(24)
        env_text += "\nJWT_SECRET=" + secrets.token_hex(32)
        env_text += "\nPOSTGRES_PASSWORD=" + secrets.token_hex(24) + "\n"
        env_text += "OLLAMA_DOCKER_URL=http://mock-ollama:11434\n"
        env_path = tmp / "test.env"
        env_path.write_text(env_text)
        env_path.chmod(0o600)
        (tmp / "mock.py").write_text(MOCK)
        overrides = ["services:"]
        for service in ["postgres","migrator","ingest","api","mcp-buddy","agent"]:
            overrides += [f"  {service}:", f"    env_file: !override [{json.dumps(str(env_path))}]",
                          "    ports: !reset []"]
            if service != "postgres":
                image = "agent" if service == "agent" else "backend"
                overrides += [f"    image: ai-buddy-{image}:integration"]
        overrides += [
            "  mock-ollama:", "    image: ai-buddy-agent:integration",
            '    command: ["python", "/mock.py"]', "    volumes:",
            f"      - {tmp / 'mock.py'}:/mock.py:ro",
        ]
        overlay = tmp / "compose.yaml"
        overlay.write_text("\n".join(overrides)+"\n")
        cmd=["docker","compose","--project-name",project,"--env-file",str(env_path),
             "-f",str(ROOT/"docker-compose.yml"),"-f",str(overlay)]
        def run(*args, **kwargs):
            return subprocess.run(cmd+list(args),cwd=ROOT,check=True,**kwargs)
        try:
            run("config","--quiet")
            run("build","api","agent")
            run("up","--no-build","-d","--wait","--wait-timeout","120","mock-ollama","agent")
            run("exec","-T","agent","python","-",input=CLIENT,text=True)
            result=run("exec","-T","postgres","psql","-U","buddy","-d","buddy",
                       "-Atc","SELECT count(*) FROM buddy.runs WHERE status='completed';",
                       capture_output=True,text=True)
            assert result.stdout.strip()=="2", result.stdout
            print("PASS: two completed runs persisted; migrations applied on fresh PostgreSQL")
        finally:
            run("down","--volumes","--remove-orphans")
if __name__ == "__main__":
    main()
