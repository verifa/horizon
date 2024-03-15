package store

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/verifa/horizon/pkg/hz"
)

type ValidateRequest struct{}

func (s *Store) Validate(ctx context.Context, req ValidateRequest) error {
	return &hz.Error{
		Status:  http.StatusNotImplemented,
		Message: "not implemented",
	}
}

func (s *Store) validateCreate(
	ctx context.Context,
	key hz.ObjectKeyer,
	data []byte,
) error {
	subject := fmt.Sprintf(
		hz.SubjectCtlrValidateCreate,
		key.ObjectGroup(),
		key.ObjectVersion(),
		key.ObjectKind(),
	)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := s.Conn.RequestWithContext(ctx, subject, data)
	if err != nil {
		return hz.ErrorFromNATSErr(err)
	}
	return hz.ErrorFromNATS(reply)
}

func (s *Store) validateUpdate(
	ctx context.Context,
	key hz.ObjectKeyer,
	data []byte,
) error {
	subject := fmt.Sprintf(
		hz.SubjectCtlrValidateUpdate,
		key.ObjectGroup(),
		key.ObjectVersion(),
		key.ObjectKind(),
	)
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	reply, err := s.Conn.RequestWithContext(ctx, subject, data)
	if err != nil {
		return hz.ErrorFromNATSErr(err)
	}
	return hz.ErrorFromNATS(reply)
}
