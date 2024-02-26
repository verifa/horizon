package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sync"
)

const (
	goCILint = "github.com/golangci/golangci-lint/cmd/golangci-lint"
	goFumpt  = "mvdan.cc/gofumpt"
	goAir    = "github.com/cosmtrek/air"
	goTempl  = "github.com/a-h/templ/cmd/templ"
)

const (
	curDir = "."
	recDir = "./..."
)

func main() {
	var build, lint, test, pr bool
	flag.BoolVar(&build, "build", false, "build the website locally")
	flag.BoolVar(&lint, "lint", false, "lint the code")
	flag.BoolVar(&test, "test", false, "run the tests")
	flag.BoolVar(&pr, "pr", false, "run the pull request checks")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Handle recover and cancel context.
	defer func() {
		if err := recover(); err != nil {
			cancel()
			log.Println("panic occurred:", err)
		}
	}()

	// Handle signals and create cancel context.
	ctx, stop := signal.NotifyContext(
		ctx,
		os.Interrupt,
		os.Kill,
	)
	defer stop()

	if lint {
		Lint(ctx)
	}
	if test {
		Test(ctx)
	}
	if build {
		panic("build: not implemented")
		// _ = KoBuild(ctx, WithKoLocal())
	}
	if pr {
		PullRequest(ctx)
	}
}

type genOptions struct {
	file         string
	ignoreErrors bool
}

type GenOption func(*genOptions)

func WithFile(f string) GenOption {
	return func(o *genOptions) {
		o.file = f
	}
}

func WithIgnoreErrors() GenOption {
	return func(o *genOptions) {
		o.ignoreErrors = true
	}
}

func Generate(ctx context.Context, opts ...GenOption) {
	fmt.Println("📝 generating files")
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		if err := TemplGenerate(ctx, opts...); err != nil {
			panic(fmt.Sprintf("templ: %s", err))
		}
		wg.Done()
	}()
	go func() {
		// Skip Tailwind generation for now.
		// iferr(TailwindGenerate(ctx))
		wg.Done()
	}()
	wg.Wait()
	fmt.Println("✅ content generated")
}

func TemplGenerate(ctx context.Context, opts ...GenOption) error {
	opt := &genOptions{}
	for _, o := range opts {
		o(opt)
	}
	args := []string{goTempl, "generate"}
	if opt.file != "" {
		args = append(args, "-f", opt.file)
	}
	if err := GoRun(ctx, args...); err != nil {
		if !opt.ignoreErrors {
			return err
		}
	}
	return nil
}

func TailwindGenerate(ctx context.Context) error {
	return NpxRun(
		ctx,
		"tailwindcss",
		"--config",
		"./tailwind.config.js",
		"--output",
		"./pkg/gateway/dist/tailwind.css",
		"--minify",
	)
}

func Lint(ctx context.Context) {
	fmt.Println("🧹 code linting")
	Generate(ctx)
	iferr(Go(ctx, "mod", "tidy"))
	iferr(Go(ctx, "mod", "verify"))
	iferr(GoRun(ctx, goFumpt, "-w", "-extra", curDir))
	iferr(GoRun(ctx, goCILint, "-v", "run", recDir))
	fmt.Println("✅ code linted")
}

func Test(ctx context.Context) {
	fmt.Println("🧪 running tests")
	Generate(ctx)
	iferr(Go(ctx, "test", "-v", recDir))
	fmt.Println("✅ tests passed")
}

func PullRequest(ctx context.Context) {
	Generate(ctx)
	Lint(ctx)
	Test(ctx)
	fmt.Println("✅ pull request checks passed")
}

func Go(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "go", args...)
	slog.Info("exec", slog.String("cmd", cmd.String()))
	defer slog.Info("done", slog.String("cmd", cmd.String()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.Stderr.Sync()
		_ = os.Stdout.Sync()
		return fmt.Errorf("go: %s", err)
	}
	return nil
}

func GoRun(ctx context.Context, args ...string) error {
	return Go(ctx, append([]string{"run", "-mod=readonly"}, args...)...)
}

func NpxRun(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "npx", args...)
	slog.Info("exec", slog.String("cmd", cmd.String()))
	defer slog.Info("done", slog.String("cmd", cmd.String()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.Stderr.Sync()
		_ = os.Stdout.Sync()
		return fmt.Errorf("npx: %w", err)
	}
	return nil
}

func DockerRun(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	slog.Info("exec", slog.String("cmd", cmd.String()))
	defer slog.Info("done", slog.String("cmd", cmd.String()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.Stderr.Sync()
		_ = os.Stdout.Sync()
		return fmt.Errorf("docker: %s", err)
	}
	return nil
}

func iferr(err error) {
	if err != nil {
		panic(err)
	}
}
