package terra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
)

type ReconcileOption func(*reconcileOptions)

func WithLogger(l *slog.Logger) ReconcileOption {
	return func(o *reconcileOptions) {
		o.slog = l
	}
}

func WithWorkdir(workdir string) ReconcileOption {
	return func(o *reconcileOptions) {
		o.workdir = workdir
	}
}

func WithExecPath(execPath string) ReconcileOption {
	return func(o *reconcileOptions) {
		o.execPath = execPath
	}
}

func WithPlanFile(planFile string) ReconcileOption {
	return func(o *reconcileOptions) {
		o.planFile = planFile
	}
}

func WithSkipApply(b bool) ReconcileOption {
	return func(o *reconcileOptions) {
		o.skipApply = b
	}
}

func WithDestroy(b bool) ReconcileOption {
	return func(o *reconcileOptions) {
		o.destroy = b
	}
}

func WithFS(fsys fs.FS) ReconcileOption {
	return func(o *reconcileOptions) {
		o.fsys = fsys
	}
}

func WithFSSub(sub string) ReconcileOption {
	return func(o *reconcileOptions) {
		o.fsysSub = sub
	}
}

func WithTFVars(tfvars any) ReconcileOption {
	return func(o *reconcileOptions) {
		o.tfvars = tfvars
	}
}

func WithJSONFile(value any, file string) ReconcileOption {
	return func(o *reconcileOptions) {
		o.jsonFiles = append(o.jsonFiles, jsonFile{
			value: value,
			file:  file,
		})
	}
}

func WithBackend(b Backender) ReconcileOption {
	return func(o *reconcileOptions) {
		o.backend = b
	}
}

func WithOutputs(outputs ...Outputer) ReconcileOption {
	return func(o *reconcileOptions) {
		o.outputs = outputs
	}
}

type reconcileOptions struct {
	slog *slog.Logger

	workdir  string
	execPath string

	planFile string

	skipApply bool
	destroy   bool

	fsys    fs.FS
	fsysSub string

	tfvars    any
	outputs   []Outputer
	backend   Backender
	jsonFiles []jsonFile
}

type jsonFile struct {
	value any
	file  string
}

func Reconcile(ctx context.Context, opts ...ReconcileOption) (*Result, error) {
	ro := reconcileOptions{
		// Discard logs by default.
		slog:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		execPath: "terraform",
		planFile: "tfplan",
	}
	for _, opt := range opts {
		opt(&ro)
	}

	if ro.fsysSub != "" {
		var err error
		ro.fsys, err = fs.Sub(ro.fsys, ro.fsysSub)
		if err != nil {
			return nil, fmt.Errorf("subbing fs: %w", err)
		}
	}

	if err := os.RemoveAll(ro.workdir); err != nil {
		return nil, fmt.Errorf("removing workdir: %w", err)
	}

	if err := os.MkdirAll(ro.workdir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("mkdirall: %w", err)
	}

	tf, err := tfexec.NewTerraform(ro.workdir, ro.execPath)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	if err := unpack(ro.workdir, ro.fsys); err != nil {
		return nil, fmt.Errorf("unpacking: %w", err)
	}

	if ro.tfvars != nil {
		tfvarsFile, err := os.Create(
			filepath.Join(ro.workdir, "terraform.tfvars.json"),
		)
		if err != nil {
			return nil, fmt.Errorf("creating tfvars file: %w", err)
		}
		if err := json.NewEncoder(tfvarsFile).Encode(ro.tfvars); err != nil {
			return nil, fmt.Errorf("encoding tfvars: %w", err)
		}
	}

	if ro.backend == nil {
		return nil, fmt.Errorf("backend required")
	}
	if err := jsonEncodeBackend(ro.workdir, ro.backend); err != nil {
		return nil, fmt.Errorf("encoding backend: %w", err)
	}

	for _, output := range ro.outputs {
		path := filepath.Join(
			ro.workdir,
			fmt.Sprintf("output-%s.tf.json", output.Name()),
		)

		f, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("creating file: %w", err)
		}
		encVal := map[string]interface{}{
			"output": map[string]interface{}{
				output.Name(): map[string]interface{}{
					"value":     fmt.Sprintf("${%s}", output.Resource()),
					"sensitive": true,
				},
			},
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(encVal); err != nil {
			return nil, fmt.Errorf("encoding: %w", err)
		}

	}
	for _, jf := range ro.jsonFiles {
		if err := jsonEncodeFile(jf.value, ro.workdir, jf.file); err != nil {
			return nil, fmt.Errorf("encoding json file: %w", err)
		}
	}

	// tf.SetStdout(os.Stdout)
	// tf.SetStderr(os.Stderr)
	return reconcile(ctx, tf, ro)
}

func reconcile(
	ctx context.Context,
	tf *tfexec.Terraform,
	opts reconcileOptions,
) (*Result, error) {
	slog.Info("running terraform init")
	if err := tf.Init(ctx, tfexec.Upgrade(true)); err != nil {
		return nil, fmt.Errorf("terraform init: %s", err)
	}

	slog.Info("running terraform plan")
	diff, err := tf.Plan(
		ctx,
		tfexec.Out(opts.planFile),
		tfexec.Destroy(opts.destroy),
	)
	if err != nil {
		return nil, fmt.Errorf("terraform plan: %w", err)
	}
	// tf.SetStdout(io.Discard)
	tfplan, err := tf.ShowPlanFile(ctx, opts.planFile)
	if err != nil {
		return nil, fmt.Errorf("terraform show plan: %w", err)
	}
	// tf.SetStdout(os.Stdout)
	result := Result{
		Plan: tfplan,
	}
	if !diff {
		slog.Info("no changes")
	} else {
		if opts.skipApply {
			slog.Info("skipping apply")
		} else {
			slog.Info("running terraform apply")
			if err := tf.Apply(ctx, tfexec.DirOrPlan("tfplan")); err != nil {
				return nil, fmt.Errorf("terraform apply: %w", err)
			}
		}
	}

	// tf.SetStdout(io.Discard)
	tfstate, err := tf.Show(ctx)
	if err != nil {
		return nil, fmt.Errorf("terraform show: %w", err)
	}
	result.State = tfstate
	// tf.SetStdout(os.Stdout)

	// Skip outputs parsing if we're destroying.
	if opts.destroy {
		return &result, nil
	}
	if len(opts.outputs) > 0 {
		tfOutputs, err := tf.Output(ctx)
		if err != nil {
			return nil, fmt.Errorf("terraform output: %w", err)
		}
		for _, o := range opts.outputs {

			v, ok := tfOutputs[o.Name()]
			if !ok {
				return nil, fmt.Errorf("output %s not found", o.Name())
			}
			if err := json.Unmarshal(v.Value, o); err != nil {
				return nil, fmt.Errorf(
					"unmarshalling output %q: %w",
					o.Name(),
					err,
				)
			}
		}
	}
	numCreate, numUpdate, numDelete := DiffCount(tfplan)
	slog.Info(
		"reconcile summary",
		"create",
		numCreate,
		"update",
		numUpdate,
		"delete",
		numDelete,
	)

	if numCreate != 0 || numUpdate != 0 || numDelete != 0 {
		rawPlan, err := tf.ShowPlanFileRaw(ctx, opts.planFile)
		if err != nil {
			return nil, fmt.Errorf("terraform show plan raw: %w", err)
		}
		fmt.Println(rawPlan)
	}
	return &result, nil
}

func unpack(workdir string, fsys fs.FS) error {
	return fs.WalkDir(
		fsys,
		".",
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			fullPath := filepath.Join(workdir, path)
			if d.IsDir() {
				if err := os.MkdirAll(fullPath, os.ModePerm); err != nil {
					return fmt.Errorf("mkdirall: %w", err)
				}
			} else {
				srcFile, err := fsys.Open(path)
				if err != nil {
					return fmt.Errorf("open: %w", err)
				}
				defer srcFile.Close()
				if err := writeFile(fullPath, srcFile); err != nil {
					return fmt.Errorf("write file: %w", err)
				}
			}
			return nil
		},
	)
}

func jsonEncodeBackend(workdir string, b Backender) error {
	encVal := map[string]interface{}{
		"terraform": map[string]interface{}{
			"backend": map[string]interface{}{
				b.Backend(): b,
			},
		},
	}
	f, err := os.Create(filepath.Join(workdir, "backend.tf.json"))
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(encVal); err != nil {
		return fmt.Errorf("encoding: %w", err)
	}
	return nil
}

func jsonEncodeFile(value any, workdir string, file string) error {
	path := filepath.Join(workdir, file)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return fmt.Errorf("encoding: %w", err)
	}
	return nil
}

func writeFile(path string, src io.Reader) error {
	dstFile, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer dstFile.Close()
	if _, err := io.Copy(dstFile, src); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}

type Result struct {
	Plan  *tfjson.Plan
	State *tfjson.State
	Phase Phase
}

type Phase string

const (
	PhasePlanNoDiff Phase = "plan_no_diff"
	PhasePlanDiff   Phase = "plan_diff"
	PhaseApply      Phase = "apply"
)
