package hz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"github.com/verifa/horizon/pkg/internal/hzcue"
)

type Validator interface {
	ValidateCreate(ctx context.Context, data []byte) error
	ValidateUpdate(ctx context.Context, old, data []byte) error
}

var _ Validator = (*ValidateNothing)(nil)

// ValidateNothing is a validator that does not validate anything but it
// implements [Validator].
// It is therefore useful to embed in other validators that only need to
// implement a subset of the [Validator] interface orr for testing.
type ValidateNothing struct{}

func (z *ValidateNothing) ValidateCreate(
	ctx context.Context,
	data []byte,
) error {
	return nil
}

func (z *ValidateNothing) ValidateUpdate(
	ctx context.Context,
	old []byte,
	data []byte,
) error {
	return nil
}

var _ Validator = (*ValidateCUE)(nil)

// ValidateCUE is a validator that uses CUE to validate objects.
type ValidateCUE struct {
	Object Objecter
	cCtx   *cue.Context
	cueDef cue.Value
}

func (v *ValidateCUE) ValidateCreate(ctx context.Context, data []byte) error {
	return v.validate(ctx, data)
}

func (v *ValidateCUE) ValidateUpdate(
	ctx context.Context,
	old []byte,
	data []byte,
) error {
	return v.validate(ctx, data)
}

func (v *ValidateCUE) ParseObject() error {
	if v.cCtx != nil {
		return errors.New("cue context already initialised")
	}
	cCtx := cuecontext.New()
	cueSpec, err := hzcue.SpecFromObject(cCtx, v.Object)
	if err != nil {
		return fmt.Errorf("generating cue spec: %w", err)
	}
	v.cCtx = cCtx
	v.cueDef = cueSpec
	return nil
}

func (v *ValidateCUE) validate(_ context.Context, data []byte) error {
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

var _ Validator = (*ValidateNamespaceRoot)(nil)

// ValidateNamespaceRoot is a validator that checks if the namespace is root.
type ValidateNamespaceRoot struct{}

func (v *ValidateNamespaceRoot) ValidateCreate(
	ctx context.Context,
	data []byte,
) error {
	return v.validateRootNamespace(data)
}

func (v *ValidateNamespaceRoot) ValidateUpdate(
	ctx context.Context,
	old []byte,
	data []byte,
) error {
	return v.validateRootNamespace(data)
}

func (v *ValidateNamespaceRoot) validateRootNamespace(
	data []byte,
) error {
	meta := MetaOnlyObject{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("unmarshal data metadata: %w", err)
	}
	if meta.Namespace != NamespaceRoot {
		return fmt.Errorf(
			"namespace must be %q but is %q",
			NamespaceRoot,
			meta.Namespace,
		)
	}
	return nil
}
