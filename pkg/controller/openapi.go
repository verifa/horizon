package controller

import (
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/encoding/openapi"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/internal/hzcue"
	"github.com/verifa/horizon/pkg/internal/openapiv3"
)

func OpenAPIV3SchemaFromObject(obj hz.Objecter) (*openapiv3.Schema, error) {
	bOpenAPI, err := openAPIV3FromObject(obj)
	if err != nil {
		return nil, err
	}
	spec := openapiv3.Spec{}
	if err := json.Unmarshal(bOpenAPI, &spec); err != nil {
		return nil, fmt.Errorf("unmarshalling open api spec: %w", err)
	}
	if len(spec.Components.Schemas) != 1 {
		return nil, fmt.Errorf(
			"expected 1 schema, got %d",
			len(spec.Components.Schemas),
		)
	}
	for key, schema := range spec.Components.Schemas {
		schema.Key = key
		return &schema, nil
	}
	return nil, fmt.Errorf("no schema")
}

func openAPIV3FromObject(obj hz.Objecter) ([]byte, error) {
	cCtx := cuecontext.New()
	cueSpec, err := hzcue.SpecFromObject(cCtx, obj)
	if err != nil {
		return nil, err
	}

	// We need to wrap the cue spec into a schema definition.
	defPath := cue.MakePath(cue.Def(obj.ObjectKind()))
	oapiSpec := cCtx.CompileString("{}").FillPath(defPath, cueSpec)
	bOpenAPI, err := openapi.Gen(oapiSpec, &openapi.Config{
		ExpandReferences: true,
	})
	if err != nil {
		return nil, fmt.Errorf("generating open api spec: %w", err)
	}
	return bOpenAPI, nil
}
