package httpapi

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// agentChatPath — путь чата на стороне агента; публичный POST /api/chat уходит сюда.
const agentChatPath = "/internal/chat"

// agentKeyHeader — заголовок, по которому агент узнаёт api.
const agentKeyHeader = "X-Agent-Key"

// NewAgentProxy возвращает SSE-прокси на агента.
// Не зависит от Server: монтируется как POST /api/chat за requireAuth.
// Authorization уходит агенту как есть (агент передаёт токен в MCP),
// X-Agent-Key клиента всегда выкидывается и заменяется нашим.
func NewAgentProxy(target *url.URL, agentKey string) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// SetURL склеивает пути; нам нужен фиксированный путь агента,
			// поэтому подменяем его поверх базового пути target.
			pr.Out.URL.Path = strings.TrimRight(target.Path, "/") + agentChatPath
			pr.Out.URL.RawPath = ""
			pr.SetXForwarded()

			// Сначала Del: клиент мог прислать несколько значений заголовка.
			pr.Out.Header.Del(agentKeyHeader)
			pr.Out.Header.Set(agentKeyHeader, agentKey)
		},
		// -1: сбрасываем каждый кусок сразу, иначе SSE-события копятся в буфере.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// Логируем только метод, путь и ошибку — без заголовков с токенами.
			log.Printf("agent proxy: %s %s: %v", r.Method, r.URL.Path, err)
			writeError(w, http.StatusBadGateway, "agent unavailable")
		},
	}
}
