package httpapi

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// seenRequest — что фейковый агент увидел во входящем запросе.
type seenRequest struct {
	path       string
	agentKeys  []string
	authHeader string
	body       string
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestAgentProxyStreamsAndRewritesHeaders(t *testing.T) {
	seen := make(chan seenRequest, 1)
	// releaseSecond открывается тестом только после того, как клиент прочитал первое событие.
	releaseSecond := make(chan struct{})

	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen <- seenRequest{
			path:       r.URL.Path,
			agentKeys:  r.Header.Values("X-Agent-Key"),
			authHeader: r.Header.Get("Authorization"),
			body:       string(body),
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		_, _ = w.Write([]byte("data: first\n\n"))
		flusher.Flush()

		select {
		case <-releaseSecond:
		case <-time.After(5 * time.Second):
			// Клиент так и не получил первое событие — прокси буферизует поток.
			return
		}
		_, _ = w.Write([]byte("data: second\n\n"))
		flusher.Flush()
	}))
	defer agent.Close()

	proxy := httptest.NewServer(NewAgentProxy(mustParseURL(t, agent.URL), "server-key"))
	defer proxy.Close()

	req, err := http.NewRequest(http.MethodPost, proxy.URL+"/api/chat", strings.NewReader(`{"message":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer user-token")
	req.Header.Add("X-Agent-Key", "client-forged-key")
	req.Header.Add("X-Agent-Key", "client-forged-key-2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	first, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read first event: %v", err)
	}
	if first != "data: first\n" {
		t.Fatalf("first line = %q", first)
	}
	// Первое событие дошло, пока второе ещё не отправлено, — поток не буферизуется.
	close(releaseSecond)

	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read rest: %v", err)
	}
	if !strings.Contains(string(rest), "data: second\n") {
		t.Fatalf("second event not received, got %q", rest)
	}

	got := <-seen
	if got.path != "/internal/chat" {
		t.Fatalf("agent path = %q, want /internal/chat", got.path)
	}
	if len(got.agentKeys) != 1 || got.agentKeys[0] != "server-key" {
		t.Fatalf("agent keys = %q, want only server key", got.agentKeys)
	}
	for _, k := range got.agentKeys {
		if strings.HasPrefix(k, "client-forged") {
			t.Fatal("client key reached the agent")
		}
	}
	if got.authHeader != "Bearer user-token" {
		t.Fatalf("authorization = %q", got.authHeader)
	}
	if got.body != `{"message":"hi"}` {
		t.Fatalf("body = %q", got.body)
	}
}

func TestAgentProxyKeepsTargetBasePath(t *testing.T) {
	paths := make(chan string, 1)
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
	}))
	defer agent.Close()

	h := NewAgentProxy(mustParseURL(t, agent.URL+"/base/"), "k")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/chat", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if p := <-paths; p != "/base/internal/chat" {
		t.Fatalf("path = %q, want /base/internal/chat", p)
	}
}

func TestAgentProxyAgentDown(t *testing.T) {
	// Поднимаем и сразу закрываем сервер: адрес гарантированно не слушается.
	agent := httptest.NewServer(http.NotFoundHandler())
	target := mustParseURL(t, agent.URL)
	agent.Close()

	h := NewAgentProxy(target, "k")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer secret-token")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "agent unavailable" {
		t.Fatalf("error = %q", body["error"])
	}
}
