#!/usr/bin/env python3
"""Write fictional Claude Code transcripts for the demo recording.

    demo/seed.py <config-dir> [--live | --live-only | --diff <repo-dir> | --ask <state-dir> <pid>]

Creates <config-dir>/projects/... in the layout Claude Code uses, so
CLAUDE_CONFIG_DIR=<config-dir> snoop shows them instead of your own sessions.
Timestamps are relative to now, so the picker's ages and the summary's
30-day chart look the same whenever the GIF is rendered. --live adds three
sessions active in the last few minutes, for the live board's screenshot;
--live-only writes just those, right before that screenshot is taken.
--diff makes a small git repository with uncommitted changes in
<repo-dir>/weather-api, and the session that made them, for the Diff tab.
"""

import json
import os
import random
import subprocess
import sys
import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path

MODEL = "claude-opus-5-5"
random.seed(7)


class Transcript:
    """Builds one transcript's records with a moving clock and context."""

    def __init__(self, cwd, branch, start, session_id=None):
        self.cwd, self.branch = cwd, branch
        self.t = start
        self.id = session_id or str(uuid.uuid4())
        self.records = []
        self.ctx = 18_000
        self.n = 0

    def _base(self, typ):
        self.n += 1
        return {"type": typ, "uuid": f"{self.id[:8]}-{self.n}", "sessionId": self.id,
                "timestamp": self.t.isoformat().replace("+00:00", "Z"),
                "cwd": self.cwd, "gitBranch": self.branch, "version": "2.1.310"}

    def wait(self, seconds):
        self.t += timedelta(seconds=seconds)

    def title(self, text):
        self.records.append({"type": "ai-title", "sessionId": self.id, "aiTitle": text})

    def prompt(self, text):
        r = self._base("user")
        r["message"] = {"role": "user", "content": text}
        self.records.append(r)
        self.wait(2)

    def _assistant(self, blocks, out_tokens):
        self.ctx += random.randint(1_500, 6_000)
        mid = "msg_" + uuid.uuid4().hex[:20]
        usage = {"input_tokens": random.randint(2, 40), "output_tokens": out_tokens,
                 "cache_read_input_tokens": self.ctx - 3_000, "cache_creation_input_tokens": 3_000}
        for b in blocks:
            r = self._base("assistant")
            r["message"] = {"id": mid, "model": MODEL, "role": "assistant", "content": [b], "usage": usage}
            self.records.append(r)
        self.wait(1)

    def think(self, text):
        self._assistant([{"type": "thinking", "thinking": text}], 220)
        self.wait(3)

    def say(self, text):
        self._assistant([{"type": "text", "text": text}], 400)
        self.wait(2)

    def tools(self, *calls):
        """Run calls in parallel: (name, input, result, seconds[, error[, agent_id]])."""
        blocks, ids = [], []
        for c in calls:
            tid = "toolu_" + uuid.uuid4().hex[:20]
            ids.append(tid)
            blocks.append({"type": "tool_use", "id": tid, "name": c[0], "input": c[1]})
        self._assistant(blocks, 180)
        start = self.t
        for tid, c in sorted(zip(ids, calls), key=lambda x: x[1][3]):
            self.t = start + timedelta(seconds=c[3])
            r = self._base("user")
            r["message"] = {"role": "user", "content": [{"type": "tool_result", "tool_use_id": tid,
                                                         "content": c[2], "is_error": len(c) > 4 and c[4]}]}
            if len(c) > 5:
                r["toolUseResult"] = {"status": "completed", "agentId": c[5]}
            self.records.append(r)
        self.wait(1)

    def call(self, name, inp):
        """Start a call that has no result yet: running, or waiting on you."""
        self._assistant([{"type": "tool_use", "id": "toolu_" + uuid.uuid4().hex[:20],
                          "name": name, "input": inp}], 120)

    def compact(self):
        r = self._base("system")
        r.update(subtype="compact_boundary", content="Conversation compacted",
                 compactMetadata={"trigger": "auto", "preTokens": self.ctx})
        self.records.append(r)
        self.ctx = 24_000
        self.wait(40)

    def end_turn(self, started):
        r = self._base("system")
        r.update(subtype="turn_duration", durationMs=int((self.t - started).total_seconds() * 1000))
        self.records.append(r)

    def cost(self, usd):
        self.records.append({"type": "cost-state", "sessionId": self.id, "totalCostUSD": usd,
                             "modelUsage": {MODEL: {"costUSD": usd}}})

    def write(self, path):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("".join(json.dumps(r) + "\n" for r in self.records))
        # The picker orders sessions by modification time.
        ts = self.t.timestamp()
        os.utime(path, (ts, ts))


def project_dir(root, cwd):
    return root / "projects" / "".join(c if c.isalnum() else "-" for c in cwd)


MIDDLEWARE = """func (s *Server) routes() {
	s.mux.HandleFunc("/forecast", s.handleForecast)
	s.mux.HandleFunc("/health", s.handleHealth)
}"""


def main_session(root, now):
    cwd = "/home/demo/src/weather-api"
    s = Transcript(cwd, "feat/rate-limit", now - timedelta(minutes=48))
    s.title("Add rate limiting to the forecast API")

    t0 = s.t
    s.prompt("Add per-client rate limiting to the /forecast endpoint. Clients are identified by API key.")
    s.think("I should look at how requests are routed and where the API key is read before adding middleware.")
    s.tools(("Grep", {"pattern": "X-API-Key", "path": "internal"}, "internal/server/auth.go:14", 0.4),
            ("Read", {"file_path": f"{cwd}/internal/server/server.go"}, MIDDLEWARE, 0.2),
            ("Read", {"file_path": f"{cwd}/internal/server/auth.go"}, "func apiKey(r *http.Request) string {...}", 0.2))
    s.tools(("Agent", {"description": "Survey existing middleware", "subagent_type": "Explore",
                       "prompt": "List every HTTP middleware in internal/ and how it is wired."},
             "Two middlewares: logging and auth, chained in server.routes().", 38, False, "a1b2c3d4"))
    s.say("Auth runs before the handler, so the limiter can key on the API key it already validated.")
    s.tools(("Write", {"file_path": f"{cwd}/internal/ratelimit/limiter.go",
                       "content": "package ratelimit\n\n// Limiter is a token bucket per API key.\ntype Limiter struct {\n\tmu      sync.Mutex\n\tbuckets map[string]*bucket\n\trate    float64\n\tburst   int\n}\n"},
             "File created", 0.1),
            ("Write", {"file_path": f"{cwd}/internal/ratelimit/limiter_test.go",
                       "content": "package ratelimit\n\nfunc TestBurstThenRefill(t *testing.T) {\n\tl := New(1, 3)\n\t// three pass, the fourth waits\n}\n"},
             "File created", 0.1))
    s.tools(("Edit", {"file_path": f"{cwd}/internal/server/server.go",
                      "old_string": '\ts.mux.HandleFunc("/forecast", s.handleForecast)',
                      "new_string": '\ts.mux.Handle("/forecast", s.limit(http.HandlerFunc(s.handleForecast)))'},
             "String not found in file", 0.1, True))
    s.tools(("Read", {"file_path": f"{cwd}/internal/server/server.go"}, MIDDLEWARE, 0.1))
    s.tools(("Edit", {"file_path": f"{cwd}/internal/server/server.go",
                      "old_string": 's.mux.HandleFunc("/forecast", s.handleForecast)',
                      "new_string": 's.mux.Handle("/forecast", s.limit(http.HandlerFunc(s.handleForecast)))'},
             "The file has been updated", 0.1))
    s.tools(("Bash", {"command": "go test ./internal/...", "description": "Run the tests"},
             "--- FAIL: TestBurstThenRefill (0.00s)\n    limiter_test.go:9: 4th request allowed", 7.2, True))
    s.think("The refill uses wall time between calls; the test needs an injectable clock.")
    s.tools(("Edit", {"file_path": f"{cwd}/internal/ratelimit/limiter.go",
                      "old_string": "\trate    float64\n\tburst   int\n}",
                      "new_string": "\trate    float64\n\tburst   int\n\tnow     func() time.Time\n}"},
             "The file has been updated", 0.1))
    s.tools(("Bash", {"command": "go test ./internal/...", "description": "Run the tests"},
             "ok  \tweather-api/internal/ratelimit\t0.012s\nok  \tweather-api/internal/server\t0.044s", 6.8),
            ("Bash", {"command": "go vet ./...", "description": "Vet"}, "", 3.1))
    s.say("Rate limiting is in: a token bucket per API key, 5 requests/second with a burst of 20, "
          "returning 429 with Retry-After. Tests cover the burst and the refill.")
    s.end_turn(t0)

    s.wait(95)
    t1 = s.t
    s.prompt("Make the limits configurable with environment variables")
    s.tools(("Grep", {"pattern": "os.Getenv", "path": "internal/config"}, "internal/config/config.go:22", 0.3))
    s.compact()
    s.tools(("Edit", {"file_path": f"{cwd}/internal/config/config.go",
                      "old_string": "type Config struct {\n\tAddr string\n}",
                      "new_string": "type Config struct {\n\tAddr      string\n\tRateLimit float64 // RATE_LIMIT, requests per second\n\tRateBurst int     // RATE_BURST\n}"},
             "The file has been updated", 0.1))
    s.tools(("Bash", {"command": "go test ./...", "description": "Run the tests"}, "ok  \tweather-api/...\t0.381s", 9.4))
    s.say("`RATE_LIMIT` and `RATE_BURST` now set the limits, defaulting to 5 and 20.")
    s.end_turn(t1)
    s.cost(4.82)
    path = project_dir(root, cwd) / f"{s.id}.jsonl"
    s.write(path)

    sub = Transcript(cwd, "feat/rate-limit", t0 + timedelta(seconds=20))
    sub.prompt("List every HTTP middleware in internal/ and how it is wired.")
    sub.tools(("Grep", {"pattern": "func.*http.Handler", "path": "internal"},
               "internal/server/log.go:9\ninternal/server/auth.go:21", 0.5))
    sub.tools(("Read", {"file_path": f"{cwd}/internal/server/log.go"}, "func logged(h http.Handler) http.Handler {...}", 0.2),
              ("Read", {"file_path": f"{cwd}/internal/server/auth.go"}, "func (s *Server) auth(h http.Handler) http.Handler {...}", 0.2))
    sub.say("Two middlewares: logging and auth, chained in server.routes().")
    sub_dir = path.with_suffix("") / "subagents"
    sub.write(sub_dir / "agent-a1b2c3d4.jsonl")
    (sub_dir / "agent-a1b2c3d4.meta.json").write_text(json.dumps(
        {"agentType": "Explore", "description": "Survey existing middleware", "finished": True}))


def history(root, now):
    """Smaller sessions over the past month, for the picker and the summary."""
    topics = [
        ("weather-api", "main", "Fix flaky cache test", ["Why does TestCacheExpiry fail on CI?"]),
        ("weather-api", "main", "Document the forecast endpoint", ["Write API docs for /forecast"]),
        ("dashboard", "main", "Chart temperature trends", ["Add a 7-day temperature chart"]),
        ("dashboard", "dark-mode", "Dark mode toggle", ["Add a dark mode toggle", "Persist it"]),
        ("infra", "main", "Terraform state cleanup", ["Remove the unused staging bucket"]),
        ("weather-api", "main", "Upgrade Go to 1.24", ["Bump Go and fix deprecations"]),
    ]
    for i in range(22):
        proj, branch, title, prompts = topics[i % len(topics)]
        cwd = f"/home/demo/src/{proj}"
        start = now - timedelta(days=random.randint(1, 29), hours=random.randint(0, 10))
        s = Transcript(cwd, branch, start)
        s.title(title)
        for p in prompts:
            t0 = s.t
            s.prompt(p)
            for _ in range(random.randint(2, 6)):
                name = random.choice(["Bash", "Read", "Edit", "Grep"])
                inp = {"Bash": {"command": "go test ./..."}, "Read": {"file_path": f"{cwd}/main.go"},
                       "Edit": {"file_path": f"{cwd}/main.go", "old_string": "a", "new_string": "b"},
                       "Grep": {"pattern": "TODO"}}[name]
                s.tools((name, inp, "ok", random.uniform(0.1, 8), random.random() < 0.06))
            if random.random() < 0.3:
                s.tools(("Skill", {"skill": random.choice(["code-review", "verify"])}, "done", 2))
            s.say("Done.")
            s.end_turn(t0)
            s.wait(120)
        s.cost(round(random.uniform(0.3, 6.5), 2))
        s.write(project_dir(root, cwd) / f"{s.id}.jsonl")


def live(root, now):
    """Sessions at work right now: one waiting on you, one done, one busy."""
    waiting = Transcript("/home/demo/src/dashboard", "main", now - timedelta(minutes=2))
    waiting.title("Paginate the device list")
    waiting.prompt("Paginate the device list, 50 per page, and add e2e tests")
    waiting.tools(("Read", {"file_path": "/home/demo/src/dashboard/src/DeviceList.tsx"}, "ok", 0.3))
    waiting.say("I'll run the e2e suite against the new pagination.")
    waiting.call("Bash", {"command": "npm run e2e -- devices"})
    waiting.write(project_dir(root, waiting.cwd) / f"{waiting.id}.jsonl")

    done = Transcript("/home/demo/src/infra", "main", now - timedelta(minutes=9))
    done.title("Rotate the CI deploy key")
    t0 = done.t
    done.prompt("Rotate the CI deploy key and update the GitHub secret")
    done.tools(("Bash", {"command": "terraform plan -target=module.ci"}, "1 to change", 6.2))
    done.tools(("Bash", {"command": "terraform apply -target=module.ci"}, "Apply complete", 11.8))
    done.say("Rotated. The new key is in the CI_DEPLOY_KEY secret.")
    done.end_turn(t0)
    done.write(project_dir(root, "/home/demo/src/infra") / f"{done.id}.jsonl")

    busy = Transcript("/home/demo/src/weather-api", "feat/alerts", now - timedelta(seconds=8))
    busy.title("Severe weather alerts")
    busy.prompt("Add a /alerts endpoint backed by the NWS feed")
    busy.tools(("Grep", {"pattern": "HandleFunc"}, "internal/server/server.go:2", 0.3))
    busy.call("Edit", {"file_path": "/home/demo/src/weather-api/internal/server/alerts.go",
                       "old_string": "", "new_string": "package server"})
    busy.write(project_dir(root, "/home/demo/src/weather-api") / f"{busy.id}.jsonl")


MIDDLEWARE_GO = """package ratelimit

import "net/http"

// Middleware rejects requests over the limit with 429 Too Many Requests.
func (l *Limiter) Middleware(key func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(key(r)) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
"""

MIDDLEWARE_GO_NEW = """package ratelimit

import (
	"net/http"
	"strconv"
)

// Middleware rejects requests over the limit with 429 Too Many Requests,
// and tells the client how long to wait.
func (l *Limiter) Middleware(key func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ok, wait := l.Reserve(key(r)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
"""

LIMITER_GO = """package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a token bucket per API key.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64
	burst   int
	now     func() time.Time
}

// Allow takes a token from key's bucket, if it has one.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucket(key)
	b.refill(l.now(), l.rate, l.burst)
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
"""

LIMITER_GO_NEW = LIMITER_GO.replace("""// Allow takes a token from key's bucket, if it has one.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucket(key)
	b.refill(l.now(), l.rate, l.burst)
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
""", """// Allow takes a token from key's bucket, if it has one.
func (l *Limiter) Allow(key string) bool {
	ok, _ := l.Reserve(key)
	return ok
}

// Reserve is Allow, and when there's no token it also says how long
// until the next one.
func (l *Limiter) Reserve(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.bucket(key)
	b.refill(l.now(), l.rate, l.burst)
	if b.tokens < 1 {
		missing := 1 - b.tokens
		return false, time.Duration(missing / l.rate * float64(time.Second))
	}
	b.tokens--
	return true, 0
}
""")

MIDDLEWARE_TEST_GO = """package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRetryAfter(t *testing.T) {
	l := New(1, 1)
	h := l.Middleware(func(*http.Request) string { return "k" }, http.NotFoundHandler())
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 429 || w.Header().Get("Retry-After") != "1" {
		t.Errorf("got %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
}
"""

README_MD = """# weather-api

Forecasts by city, from the NWS feed.

## Configuration

| Variable | Default |
| --- | --- |
| `ADDR` | `:8080` |
"""

README_MD_NEW = README_MD + """| `RATE_LIMIT` | `5` |
| `RATE_BURST` | `20` |
"""


def diff_session(root, repo_dir, now):
    """A repository with uncommitted changes, most of them the session's."""
    cwd = str(repo_dir / "weather-api")
    repo = Path(cwd)
    files = {"internal/ratelimit/middleware.go": MIDDLEWARE_GO,
             "internal/ratelimit/limiter.go": LIMITER_GO, "README.md": README_MD}
    for name, text in files.items():
        (repo / name).parent.mkdir(parents=True, exist_ok=True)
        (repo / name).write_text(text)

    def git(*args):
        subprocess.run(["git", "-C", cwd, "-c", "user.name=demo", "-c", "user.email=demo@example.com",
                        "-c", "commit.gpgsign=false", *args], check=True, capture_output=True)

    git("init", "-q")
    git("add", ".")
    git("commit", "-qm", "Rate limit the forecast endpoint")

    s = Transcript(cwd, "feat/rate-limit", now - timedelta(minutes=3))
    s.title("Send Retry-After with 429 responses")
    t0 = s.t
    s.prompt("When a client is rate limited, tell it when to retry.")
    s.tools(("Read", {"file_path": f"{cwd}/internal/ratelimit/middleware.go"}, MIDDLEWARE_GO, 0.2),
            ("Read", {"file_path": f"{cwd}/internal/ratelimit/limiter.go"}, LIMITER_GO, 0.2))
    s.tools(("Edit", {"file_path": f"{cwd}/internal/ratelimit/limiter.go",
                      "old_string": LIMITER_GO, "new_string": LIMITER_GO_NEW}, "The file has been updated", 0.1))
    s.tools(("Edit", {"file_path": f"{cwd}/internal/ratelimit/middleware.go",
                      "old_string": MIDDLEWARE_GO, "new_string": MIDDLEWARE_GO_NEW}, "The file has been updated", 0.1))
    s.tools(("Write", {"file_path": f"{cwd}/internal/ratelimit/middleware_test.go",
                       "content": MIDDLEWARE_TEST_GO}, "File created", 0.1))
    s.tools(("Bash", {"command": "go test ./internal/ratelimit", "description": "Run the tests"},
             "ok  \tweather-api/internal/ratelimit\t0.009s", 4.1))
    s.say("Rate-limited responses now carry Retry-After, in whole seconds until the next token.")
    s.end_turn(t0)
    s.cost(0.61)

    # The session's edits, and one of yours in the README.
    (repo / "internal/ratelimit/limiter.go").write_text(LIMITER_GO_NEW)
    (repo / "internal/ratelimit/middleware.go").write_text(MIDDLEWARE_GO_NEW)
    (repo / "internal/ratelimit/middleware_test.go").write_text(MIDDLEWARE_TEST_GO)
    (repo / "README.md").write_text(README_MD_NEW)
    s.write(project_dir(root, cwd) / f"{s.id}.jsonl")


def ask(root, state, pid, now):
    """A permission request from the waiting live session, as snoop's hook
    writes it, for the board's screenshot of a session asking. pid stands
    in for the hook, which must be running for snoop to show the request."""
    path = next(p for p in Path(root, "projects").rglob("*.jsonl") if "Paginate the device list" in p.read_text())
    session = path.stem
    d = Path(state, "snoop", "asks", session)
    d.mkdir(parents=True, exist_ok=True)
    stamp = lambda t: t.strftime("%Y-%m-%dT%H:%M:%S.%fZ")
    req = {"session_id": session, "cwd": "/home/demo/src/dashboard", "tool_name": "Bash",
           "tool_input": {"command": "npm run e2e -- devices",
                          "description": "Run the device list e2e tests"},
           "asked": stamp(now), "deadline": stamp(now + timedelta(minutes=5))}
    nanos = int(now.timestamp()) * 1_000_000_000
    (d / f"{nanos:020d}-{pid}.json").write_text(json.dumps(req))


if __name__ == "__main__":
    mode = sys.argv[2:]
    if mode[:1] == ["--ask"] and len(mode) == 3:
        ask(Path(sys.argv[1]), Path(mode[1]), int(mode[2]), datetime.now(timezone.utc))
        sys.exit()
    if mode[:1] == ["--diff"] and len(mode) == 2:
        diff_session(Path(sys.argv[1]), Path(mode[1]), datetime.now(timezone.utc))
        sys.exit()
    if len(sys.argv) not in (2, 3) or mode not in ([], ["--live"], ["--live-only"]):
        sys.exit("usage: demo/seed.py <config-dir> [--live | --live-only | --diff <repo-dir>]")
    root = Path(sys.argv[1])
    now = datetime.now(timezone.utc)
    if mode != ["--live-only"]:
        history(root, now)
        main_session(root, now)
    if mode:
        live(root, now)
