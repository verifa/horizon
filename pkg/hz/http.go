package hz

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"

	"github.com/nats-io/nats.go"
)

var (
	_ http.ResponseWriter = (*NATSResponseWriter)(nil)
	_ http.Flusher        = (*NATSResponseWriter)(nil)
)

type NATSResponseWriter struct {
	conn    *nats.Conn
	subject string
	resp    http.Response
	buf     bytes.Buffer
}

// Header implements http.ResponseWriter.
func (r *NATSResponseWriter) Header() http.Header {
	if r.resp.Header == nil {
		r.resp.Header = http.Header{}
	}
	return r.resp.Header
}

// Write implements http.ResponseWriter.
// It writes the bytes to an http.Response and sends it to the client.
func (r *NATSResponseWriter) Write(b []byte) (int, error) {
	// Set a default status code if one was not already set.
	if r.resp.StatusCode == 0 {
		r.resp.StatusCode = http.StatusOK
	}
	if r.resp.Header == nil {
		r.resp.Header = http.Header{}
	}
	// Set a default content type if one was not already set.
	if r.resp.Header.Get("Content-Type") == "" {
		r.resp.Header.Set("Content-Type", http.DetectContentType(b))
	}

	return r.buf.Write(b)
}

// WriteHeader implements http.ResponseWriter.
func (r *NATSResponseWriter) WriteHeader(statusCode int) {
	r.resp.StatusCode = statusCode
}

// Flush sends any buffered data to the client.
func (r *NATSResponseWriter) Flush() {
	// Make sure a status code is set. Default to 200.
	if r.resp.StatusCode == 0 {
		r.resp.StatusCode = http.StatusOK
	}

	// Add bytes to response body.
	r.resp.Body = io.NopCloser(&r.buf)

	var flushBuf bytes.Buffer
	// Write the response to the flush buffer.
	if err := r.resp.Write(&flushBuf); err != nil {
		slog.Error("writing response to flush buffer", "error", err)
		return
	}
	_ = r.conn.Publish(r.subject, flushBuf.Bytes())
	r.buf.Reset()
}
