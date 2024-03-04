package store

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/hz"
)

type ValidateRequest struct{}

func (s Store) Validate(ctx context.Context, req ValidateRequest) error {
	return errors.New("TODO")
}

func (s Store) validate(
	ctx context.Context,
	key hz.ObjectKeyer,
	data []byte,
) error {
	subject := fmt.Sprintf(
		hz.SubjectCtlrValidate,
		key.ObjectGroup(),
		key.ObjectVersion(),
		key.ObjectKind(),
	)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := s.Conn.RequestWithContext(ctx, subject, data)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return &hz.Error{
				Status:  http.StatusBadGateway,
				Message: fmt.Sprintf("no responders for %q", subject),
			}
		}
		return &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("requesting %q: %s", subject, err.Error()),
		}
	}

	if len(reply.Data) == 0 {
		return nil
	}

	return hz.ErrorFromNATS(reply)
}
