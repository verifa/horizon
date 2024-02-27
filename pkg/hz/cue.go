package hz

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
)

func cueSpecFromObject(cCtx *cue.Context, obj Objecter) (cue.Value, error) {
	cueVal := cCtx.CompileString("{}")

	kindPath := cue.ParsePath("kind")
	apiVersionPath := cue.ParsePath("apiVersion")

	// Use regular expressions for exact match string for kind, group and
	// apiVersion.
	// This is a limitatin of CUE, see this issue:
	// 	https://github.com/cue-lang/cue/issues/740
	kindExpr := cCtx.CompileString(fmt.Sprintf("=~\"^%s$\"", obj.ObjectKind()))
	if err := kindExpr.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("compiling kind expression: %w", err)
	}
	apiVersionExpr := cCtx.CompileString(
		fmt.Sprintf("=~\"^%s/%s$\"", obj.ObjectGroup(), obj.ObjectAPIVersion()),
	)
	if err := apiVersionExpr.Err(); err != nil {
		return cue.Value{}, fmt.Errorf(
			"compiling apiVersion expression: %w",
			err,
		)
	}
	t := reflect.TypeOf(obj)
	cueObj, err := cueEncodeStruct(cCtx, t)
	if err != nil {
		return cue.Value{}, fmt.Errorf("encoding struct: %w", err)
	}
	objDef := cueVal.FillPath(apiVersionPath, apiVersionExpr).
		FillPath(kindPath, kindExpr).
		Unify(cueObj)

	// We need a cue definition to validate that there are not extra fields in
	// the object.
	// To create a definition in Go, the only hacky approach I found was to
	// fill a definition path with the object and then lookup that path
	// afterwards.
	defPath := cue.MakePath(cue.Def(obj.ObjectKind()))
	cueDef := cCtx.CompileString("").
		FillPath(defPath, objDef).LookupPath(defPath)
	if cueDef.Err() != nil {
		return cue.Value{}, fmt.Errorf(
			"compiling cue definition: %w",
			cueDef.Err(),
		)
	}
	return cueDef, nil
}

func cueEncodeStruct(cCtx *cue.Context, t reflect.Type) (cue.Value, error) {
	if t.Kind() == reflect.Ptr {
		return cueEncodeStruct(cCtx, t.Elem())
	}
	if t.Kind() != reflect.Struct {
		return cue.Value{}, errors.New("value must be a struct")
	}

	// Handle special case of struct with one field which is embedded, like
	// [Time] in the ObjectMeta struct.
	if t.NumField() == 1 && t.Field(0).Anonymous {
		return cueEncodeStruct(cCtx, t.Field(0).Type)
	}

	// Handle special structs.
	iVal := reflect.New(t).Elem().Interface()
	if _, ok := iVal.(time.Time); ok {
		// This was the best attempt at getting formatting for time, but it
		// involves importing stuff and complicated things a lot right now.
		// 	importTime := ast.NewImport(nil, "time")
		// 	vtime := &ast.Ident{Name: "time", Node: importTime}
		// 	return cCtx.BuildExpr(ast.NewCall(ast.NewSel(vtime, "Format"),
		// 	ast.NewSel(vtime, "RFC3339"))), nil
		//
		// So we just go with string for now.
		return cCtx.BuildExpr(ast.NewIdent("string")), nil
	}

	val := cCtx.CompileString("{}")
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// Skip fields that are not exported as they won't be JSON marshalled
		// anyway.
		if !field.IsExported() {
			continue
		}
		fieldType := field.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		fieldPath, ok := cueFieldPath(field)
		if !ok {
			continue
		}

		// If the field is embedded / annoymous, we need to honour any JSON
		// tags, otherwise we unify it with it's parent.
		if field.Anonymous {
			jTag, ok := field.Tag.Lookup("json")
			// If no json tag, or a json tag with an empty name.
			if !ok || strings.Split(jTag, ",")[0] == "" {
				embedVal, err := cueEncodeStruct(cCtx, fieldType)
				if err != nil {
					return cue.Value{}, fmt.Errorf(
						"encoding embedded struct %q: %w",
						field.Name,
						err,
					)
				}
				val = val.Unify(embedVal)
				continue
			}
		}

		fieldExpr, err := cueEncodeField(cCtx, fieldType)
		if err != nil {
			return cue.Value{}, fmt.Errorf(
				"encoding field %q: %w",
				field.Name,
				err,
			)
		}
		fieldVal := fieldExpr
		cTag, ok := field.Tag.Lookup("cue")
		if ok {
			parts := strings.Split(cTag, ",")
			if parts[0] != "" {
				cueExpr := cCtx.CompileString(parts[0])
				if err := cueExpr.Err(); err != nil {
					return cue.Value{}, fmt.Errorf(
						"compiling cue tag %q: %w",
						cTag,
						err,
					)
				}
				fieldVal = fieldVal.Unify(cueExpr)
			}
		}
		val = val.FillPath(fieldPath, fieldVal)
	}

	return val, nil
}

func cueEncodeField(
	cCtx *cue.Context,
	fieldType reflect.Type,
) (cue.Value, error) {
	if fieldType.Kind() == reflect.Ptr {
		return cueEncodeField(cCtx, fieldType.Elem())
	}

	// Handle special types.
	iVal := reflect.New(fieldType).Elem().Interface()
	if _, ok := iVal.(json.RawMessage); ok {
		// Use lattice type for raw message.
		return cCtx.BuildExpr(ast.NewIdent("_")), nil
	}

	switch fieldType.Kind() {
	case reflect.Struct:
		return cueEncodeStruct(cCtx, fieldType)

	// Had an error treating arrays differently when generating the OpenAPI
	// spec, from cue. So just treat them like slices... sigh.
	// case reflect.Array:
	// 	// An array in Go, `[3]string` becomes simply `[string, string, string]`
	// 	// in CUE.
	// 	// So just add the field element to the list as many times as the array
	// 	// is long.
	// 	elem := fieldType.Elem()
	// 	elemVal, err := cueEncodeField(cCtx, field, elem)
	// 	if err != nil {
	// 		return cue.Value{}, err
	// 	}
	// 	vals := make([]cue.Value, fieldType.Len())
	// 	for i := 0; i < fieldType.Len(); i++ {
	// 		vals[i] = elemVal
	// 	}
	// 	return cCtx.NewList(vals...), nil
	case reflect.Slice, reflect.Array:
		elem := fieldType.Elem()
		elemVal, err := cueEncodeField(cCtx, elem)
		if err != nil {
			return cue.Value{}, err
		}
		// This is a bit hacky.
		// We need to add the ellipsis (...) in front of the element of the
		// list.
		// There seems to be no simple way of doing this without using the
		// ast.Ellipsis{} struct, but that requires working with ast.Expr, which
		// is a bit low-level and involved.
		// Hence, we just manually create [...<bytes>], where <bytes> is the raw
		// bytes of the element. We then compile that back into a cue.Value.
		// Not lovely, not efficient, but practical.
		node := elemVal.Syntax()
		b, err := format.Node(node)
		if err != nil {
			return cue.Value{}, err
		}

		buf := bytes.Buffer{}
		buf.WriteByte('[')
		buf.Write([]byte("..."))
		buf.Write(b)
		buf.WriteByte(']')
		listVal := cCtx.CompileBytes(buf.Bytes())
		return listVal, nil
	case reflect.Map:

		iVal := reflect.New(fieldType).Elem().Interface()
		// Encode type returns an or expression like:
		// 	*null | {
		// 		[string]: string
		// 	}
		// We don't care about the null, and the other value is a struct, so
		// take the first struct value.
		mapVal := cCtx.EncodeType(iVal)
		op, vals := mapVal.Expr()
		if op != cue.OrOp {
			return cue.Value{}, fmt.Errorf(
				"encoding map: expected or expression, got %s",
				op,
			)
		}
		for _, val := range vals {
			if val.IncompleteKind() == cue.StructKind {
				return val, nil
			}
		}
		return cue.Value{}, errors.New(
			"using cue to encode the map did not produce a struct value",
		)
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Float32,
		reflect.Float64,
		reflect.Bool,
		reflect.String:
		iVal := reflect.New(fieldType).Elem().Interface()
		return cCtx.EncodeType(iVal), nil
	case reflect.Interface:
		return cue.Value{}, errors.New("interface type not supported")
	default:
		return cue.Value{}, fmt.Errorf(
			"unsupported type %s",
			fieldType.Kind(),
		)
	}
}

func cueFieldPath(field reflect.StructField) (cue.Path, bool) {
	fieldName := field.Name
	isRequired := false
	jTag, ok := field.Tag.Lookup("json")
	if ok {
		fieldName = strings.Split(jTag, ",")[0]
	}
	cTag, ok := field.Tag.Lookup("cue")
	if ok {
		parts := strings.Split(cTag, ",")
		if len(parts) == 1 || parts[1] != "opt" {
			isRequired = true
		}
		if cTag == "-" {
			return cue.Path{}, false
		}
	}
	fieldPath := cue.ParsePath(fieldName)
	if !isRequired {
		fieldPath = fieldPath.Optional()
	}
	return fieldPath, true
}
