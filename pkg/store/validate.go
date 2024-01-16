package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/hz"
)

type ValidateRequest struct{}

func (s Store) Validate(ctx context.Context, req ValidateRequest) error {
	return errors.New("TODO")
}

func (s Store) validate(ctx context.Context, kind string, data []byte) error {
	subject := fmt.Sprintf("CTLR.validate.%s", kind)
	slog.Info("validate", "subject", subject)
	reply, err := s.conn.Request(subject, data, time.Second)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return errors.New("controller not responding")
		}
		return fmt.Errorf("request: %w", err)
	}

	if len(reply.Data) == 0 {
		return nil
	}

	var vErr hz.Error
	vErr.Status, err = strconv.Atoi(reply.Header.Get(hz.HeaderStatus))
	if err != nil {
		return fmt.Errorf("invalid status header: %w", err)
	}
	vErr.Message = string(reply.Data)
	return &vErr
}
