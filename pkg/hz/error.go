package hz

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/nats-io/nats.go"
)

var _ error = (*Error)(nil)

type Error struct {
	// Status is the HTTP status code applicable to this problem.
	Status  int    `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s (status %d)", e.Message, e.Status)
}

// respondError responds to a NATS message with an error.
// It expects the err to be an *Error and will use the status and message for
// the response.
// If not, it will use a http.StatusInternalServerError.
func RespondError(
	msg *nats.Msg,
	err error,
) error {
	if msg.Reply == "" {
		return errors.New("no reply subject")
	}
	text := err.Error()
	status := http.StatusInternalServerError

	var respErr *Error
	if errors.As(err, &respErr) {
		status = respErr.Status
	}
	response := nats.NewMsg(msg.Reply)
	response.Data = []byte(text)
	response.Header = make(nats.Header)
	response.Header.Add(HeaderStatus, fmt.Sprintf("%d", status))
	return msg.RespondMsg(response)
}

func RespondOK(
	msg *nats.Msg,
	body []byte,
) error {
	if msg.Reply == "" {
		return errors.New("no reply subject")
	}
	response := nats.NewMsg(msg.Reply)
	response.Data = body
	response.Header = make(nats.Header)
	response.Header.Add(HeaderStatus, fmt.Sprintf("%d", http.StatusOK))
	return msg.RespondMsg(response)
}
