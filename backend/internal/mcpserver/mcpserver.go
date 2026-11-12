package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options — всё, что отличает один MCP-сервер от другого.
type Options struct {
	Addr            string                          // адрес для http.Server, например ":8080"
	MCPServer       *mcp.Server                     // сервер с уже зарегистрированными tools
	Middleware      func(http.Handler) http.Handler // авторизация для /mcp; nil — без неё
	ShutdownTimeout time.Duration                   // сколько ждать текущие запросы при остановке
}

// Run запускает HTTP-сервер и блокируется, пока ctx не отменён
// или сервер не упал. nil — штатная остановка.
func Run(ctx context.Context, opts Options) error {
	var handler http.Handler = mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return opts.MCPServer }, nil)
	if opts.Middleware != nil {
		handler = opts.Middleware(handler)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", logRequests(handler))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := http.Server{
		Addr:              opts.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 1)

	go func() {
		if err := server.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				errs <- err
			}
		}
	}()

	log.Printf("listening on %s", opts.Addr)

	select {
	case v := <-errs:
		return v
	case <-ctx.Done():
	}

	//graceful shutdown, шатдаун сервера, где он дожидается выполнения текущих запросов
	shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	return nil
}

// logRequests логирует метод, путь и длительность каждого запроса.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
