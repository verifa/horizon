package hz

import (
	"encoding/json"
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/encoding/openapi"
	tu "github.com/verifa/horizon/pkg/testutil"
)

type cueObj struct {
	ObjectMeta `json:"metadata,omitempty" cue:""`
	Spec       cueSpec   `json:"spec"`
	Status     cueStatus `json:"status"`
}

func (s cueObj) ObjectKind() string {
	return "CueObject"
}

func (s cueObj) ObjectGroup() string {
	return "CueGroup"
}

func (s cueObj) ObjectVersion() string {
	return "v1"
}

type cueSpec struct {
	CueEmbed

	CueEmbedJSON `json:"embedJSON" cue:""`

	RequiredString string  `json:"requiredString" cue:""`
	RegexString    string  `json:"regexString" cue:"=~\"^[a-z]+$\""`
	PtrString      *string `json:"ptrString,omitempty" cue:"=~\"^[a-z]+$\",opt"`
	RequiredInt    int     `json:"requiredInt" cue:""`
	OptInt         int     `json:"optInt,omitempty" cue:",opt"`
	LimitsInt      int     `json:"limitsInt,omitempty" cue:"<=5"`

	Array         [3]string         `json:"array" cue:""`
	RequiredSlice []string          `json:"requiredSlice" cue:""`
	Children      []*cueStructChild `json:"children" cue:""`

	StringMap map[string]string `json:"stringMap" cue:""`
	IntMap    map[int]int       `json:"intMap" cue:""`

	RawData json.RawMessage `json:"rawData" cue:",opt"`
}

type cueStatus struct {
	Result bool `json:"result"`
}

type CueEmbed struct {
	EmbedField string `json:"embedField" cue:""`
}

type CueEmbedJSON struct {
	EmbedField string `json:"embedField" cue:""`
}

type cueStructChild struct {
	RequiredString string `json:"requiredString" cue:""`
}

func TestCueDefinition(t *testing.T) {
	expCueStr := `
{
	_#def
	_#def: {
		apiVersion: =~"^CueGroup/v1$"
		kind: =~"^CueObject$"
		metadata: {
			name:    =~"^[a-zA-Z0-9-_]+$"
			account: =~"^[a-zA-Z0-9-_]+$"
			labels?: {
				[string]: string
			}
			revision?: uint64
			ownerReferences?: [...{
				group:   string
				version: string
				kind:    string
				account: string
				name:    string
			}]
			managedFields?: [...{
				manager:    =~"^[a-zA-Z0-9-_]+$"
				fieldsType: =~"^FieldsV1$"
				fieldsV1: _
			}]
			finalizers?: [...string]
		}
		spec?: {
			embedField: string
			embedJSON: {
				embedField: string
			}
			requiredString: string
			regexString:    =~"^[a-z]+$"
			ptrString?:     =~"^[a-z]+$"
			requiredInt:    int64
			optInt?:        int64
			limitsInt:      int64 & <=5

			array: [...string]
			requiredSlice: [...string]
			children: [...{
				requiredString: string
			}]

			stringMap: {
				[string]: string
			}
			intMap: {
				[string]: int & >=-9223372036854775808 & <=9223372036854775807
			}

			rawData?: _
		}
		status?: {
			result?: bool
		}
	}
}
	`

	cCtx := cuecontext.New()

	// testType := cCtx.EncodeType(cueStruct{})
	// testRaw := cueValToBytes(t, testType)
	// fmt.Println(string(testRaw))

	cueDef, err := cueSpecFromObject(cCtx, cueObj{})
	tu.AssertNoError(t, err)
	tu.AssertNoError(t, cueDef.Err())
	tu.AssertNoError(t, cueDef.Validate(cue.All()))

	expCue := cCtx.CompileString(expCueStr)
	tu.AssertNoError(t, expCue.Err())
	cueDefRaw := cueValToBytes(t, cueDef)
	expCueRaw := cueValToBytes(t, expCue)

	// fmt.Println(string(cueDefRaw))
	// fmt.Println(string(expCueRaw))

	tu.AssertNoError(t, cueDef.Err())
	tu.AssertNoError(t, cueDef.Validate(cue.All()))

	d := cCtx.CompileString("{}").
		FillPath(cue.MakePath(cue.Def("Whatever")), cueDef)
	oapi, err := openapi.Gen(d, &openapi.Config{
		ExpandReferences: true,
	})
	tu.AssertNoError(t, err)
	fmt.Println(string(oapi))

	diff := tu.Diff(cueDefRaw, expCueRaw)
	if diff != "" {
		t.Errorf("cue definition mismatch:\n%s", diff)
	}
}

func cueValToBytes(t *testing.T, val cue.Value) []byte {
	raw, err := format.Node(val.Syntax())
	tu.AssertNoError(t, err)
	return raw
}
