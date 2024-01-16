package hz

import (
	"bytes"
	"io"
	"net/http"
)

var _ (http.ResponseWriter) = (*ResponseRecorder)(nil)

type ResponseRecorder struct {
	resp http.Response
	buf  bytes.Buffer
}

// Header implements http.ResponseWriter.
func (r *ResponseRecorder) Header() http.Header {
	if r.resp.Header == nil {
		r.resp.Header = http.Header{}
	}
	return r.resp.Header
}

// Write implements http.ResponseWriter.
func (r *ResponseRecorder) Write(b []byte) (int, error) {
	if r.resp.StatusCode == 0 {
		r.resp.StatusCode = http.StatusOK
	}
	return r.buf.Write(b)
}

// WriteHeader implements http.ResponseWriter.
func (r *ResponseRecorder) WriteHeader(statusCode int) {
	r.resp.StatusCode = statusCode
}

func (r *ResponseRecorder) Response() ([]byte, error) {
	// Set a default status code if one was not already set.
	if r.resp.StatusCode == 0 {
		r.resp.StatusCode = http.StatusOK
	}
	r.resp.Body = io.NopCloser(&r.buf)
	var tmp bytes.Buffer
	if err := r.resp.Write(&tmp); err != nil {
		return nil, err
	}
	return tmp.Bytes(), nil
}
