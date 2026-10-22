package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/server"
)

type mcpWriter struct {
	w http.ResponseWriter
}

func (m *mcpWriter) Header() http.Header         { return m.w.Header() }
func (m *mcpWriter) Write(b []byte) (int, error) { return m.w.Write(b) }
func (m *mcpWriter) WriteHeader(statusCode int)  { m.w.WriteHeader(statusCode) }
func (m *mcpWriter) Flush()                      {}

func WrapMCP(srv *server.MCPServer) gin.HandlerFunc {
	handler := server.NewStreamableHTTPServer(srv)
	return func(c *gin.Context) {
		handler.ServeHTTP(&mcpWriter{w: c.Writer}, c.Request)
	}
}
