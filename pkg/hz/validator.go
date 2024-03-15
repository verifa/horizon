package hz

import (
	"context"
	"errors"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
)

type Validator interface {
	ValidateCreate(ctx context.Context, data []byte) error
	ValidateUpdate(ctx context.Context, old, data []byte) error
	ValidateDelete(ctx context.Context, data []byte) error
}

var _ Validator = (*ZeroValidator)(nil)

type ZeroValidator struct{}

func (z *ZeroValidator) ValidateCreate(ctx context.Context, data []byte) error {
	return nil
}

func (z *ZeroValidator) ValidateUpdate(
	ctx context.Context,
	old []byte,
	data []byte,
) error {
	return nil
}

func (z *ZeroValidator) ValidateDelete(ctx context.Context, data []byte) error {
	return nil
}

var _ Validator = (*CUEValidator)(nil)

type CUEValidator struct {
	Object Objecter
	cCtx   *cue.Context
	cueDef cue.Value
}

func (v *CUEValidator) ValidateCreate(ctx context.Context, data []byte) error {
	return v.validate(ctx, data)
}

func (v *CUEValidator) ValidateUpdate(
	ctx context.Context,
	old []byte,
	data []byte,
) error {
	return v.validate(ctx, data)
}

func (v *CUEValidator) ValidateDelete(ctx context.Context, data []byte) error {
	return nil
}

func (v *CUEValidator) ParseObject() error {
	if v.cCtx != nil {
		return errors.New("cue context already initialised")
	}
	cCtx := cuecontext.New()
	cueSpec, err := cueSpecFromObject(cCtx, v.Object)
	if err != nil {
		return fmt.Errorf("generating cue spec: %w", err)
	}
	v.cCtx = cCtx
	v.cueDef = cueSpec
	return nil
}

func (v *CUEValidator) validate(_ context.Context, data []byte) error {
	if v.cCtx == nil {
		err := v.ParseObject()
		if err != nil {
			return fmt.Errorf("parsing object: %w", err)
		}
	}
	// // Debugging code.
	// // Keep it here because it is useful for testing.
	// node := v.cueDef.Syntax()
	// raw, _ := format.Node(node)
	// fmt.Println(string(raw))
	// pretty := bytes.Buffer{}
	// err := json.Indent(&pretty, data, "", "  ")
	// if err != nil {
	// 	return fmt.Errorf("indenting json: %w", err)
	// }
	// fmt.Println(pretty.String())

	cueData := v.cCtx.CompileBytes(data)
	if err := cueData.Validate(); err != nil {
		return fmt.Errorf("compiling data to cue value: %w", err)
	}
	result := v.cueDef.Unify(cueData)
	if err := result.Validate(
		cue.Final(),
		cue.Concrete(true),
		cue.Definitions(true),
		cue.Hidden(true),
		cue.Optional(true),
	); err != nil {
		var cErrs cueerrors.Error
		if errors.As(err, &cErrs) {
			return fmt.Errorf(
				"invalid data: %w",
				err,
			)
		}
		return fmt.Errorf("validating cue value: %w", err)
	}
	return nil
}
