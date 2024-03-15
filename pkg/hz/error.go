package hz

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

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

func (e *Error) Is(err error) bool {
	target, ok := err.(*Error)
	if !ok {
		return false
	}
	return e.Status == target.Status && e.Message == target.Message
}

func ErrorFromNATSErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, nats.ErrTimeout),
		errors.Is(err, context.DeadlineExceeded):
		return &Error{
			Status:  http.StatusGatewayTimeout,
			Message: fmt.Sprintf("nats timeout: %s", err.Error()),
		}
	case errors.Is(err, nats.ErrNoResponders):
		return &Error{
			Status:  http.StatusBadGateway,
			Message: fmt.Sprintf("no responders: %s", err.Error()),
		}
	}
	return &Error{
		Status:  http.StatusInternalServerError,
		Message: fmt.Sprintf("nats error: %s", err.Error()),
	}
}

func ErrorFromNATS(msg *nats.Msg) error {
	headerStatus := msg.Header.Get(HeaderStatus)
	status, err := strconv.Atoi(headerStatus)
	if err != nil {
		return fmt.Errorf("invalid status header %q: %w", headerStatus, err)
	}
	if status == http.StatusOK {
		return nil
	}
	return &Error{
		Status:  status,
		Message: string(msg.Data),
	}
}

func ErrorFromHTTP(resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK &&
		resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &Error{
			Status:  resp.StatusCode,
			Message: fmt.Sprintf("reading body: %s", err.Error()),
		}
	}
	return &Error{
		Status:  resp.StatusCode,
		Message: string(body),
	}
}

// ErrorWrap takes an error and checks if it is an [Error].
// If it is, it will make a copy of the [Error], add the given message and
// return it. The status will remain the same.
//
// If it is not an [Error], it will wrap the given error in an [Error] with the
// given status and message.
func ErrorWrap(
	err error,
	status int,
	message string,
) error {
	if err == nil {
		return nil
	}
	var hErr *Error
	if errors.As(err, &hErr) {
		return &Error{
			Status:  hErr.Status,
			Message: fmt.Sprintf("%s: %s", message, hErr.Message),
		}
	}
	return &Error{
		Status:  status,
		Message: fmt.Sprintf("%s: %s", message, err.Error()),
	}
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
		text = respErr.Message
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
