package server

import (
	"context"
	"fmt"
	"net/http"

	configinfra "github.com/duchoang/llmpool/internal/infra/config"
	"github.com/gin-gonic/gin"
)

type HTTPServer struct {
	server *http.Server
}

func NewHTTPServer(cfg configinfra.ServerConfig, handler *gin.Engine) *HTTPServer {
	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	return &HTTPServer{
		server: &http.Server{
			Addr:         address,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
	}
}

func (h *HTTPServer) Start() error {
	return h.server.ListenAndServe()
}

func (h *HTTPServer) Shutdown(ctx context.Context) error {
	return h.server.Shutdown(ctx)
}
