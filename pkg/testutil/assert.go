package testutil

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// AssertNoError checks if the error is nil.
func AssertNoError(t *testing.T, err error, msg ...string) {
	t.Helper()
	if err != nil {
		t.Fatal(Callers(), err, msg)
	}
}

// AssertErrorIs checks if the error is another error.
func AssertErrorIs(t *testing.T, err error, expErr error, msg ...string) {
	t.Helper()
	if err == nil {
		t.Fatal(Callers(), msg, errors.New("error was expected but is nil"))
	}
	if !errors.Is(err, expErr) {
		t.Fatal(
			Callers(),
			msg,
			fmt.Errorf("expected error %##v but got %##v", expErr, err),
		)
	}
}

// AsserterrorAs checks if the error is another error.
func AssertErrorAs[T any](t *testing.T, err error, msg ...string) {
	t.Helper()
	if err == nil {
		t.Fatal(Callers(), msg, errors.New("error was expected but is nil"))
	}
	var v T
	if !errors.As(err, &v) {
		t.Fatal(
			Callers(),
			msg,
			fmt.Errorf("error not of expected type: %##v", err),
		)
	}
}

// AssertEqual checks if the expected and actual are equal. Errors if not.
func AssertEqual(
	t *testing.T,
	expected, actual interface{},
	opts ...cmp.Option,
) {
	t.Helper()
	if diff := Diff(actual, expected, opts...); diff != "" {
		t.Fatal(Callers(), diff)
	}
}

// AssertTrue checks if the condition is true. Errors if not.
func AssertTrue(t *testing.T, condition bool, msg ...string) {
	t.Helper()
	if !condition {
		t.Fatal("expected true, got false: ", msg)
	}
}

// Diff compares two items and returns a human-readable diff string. If the
// items are equal, the string is empty.
func Diff[T any](got, want T, opts ...cmp.Option) string {
	// nolint: gocritic
	oo := append(
		opts,
		cmp.Exporter(func(reflect.Type) bool { return true }),
		cmpopts.EquateEmpty(),
	)

	diff := cmp.Diff(got, want, oo...)
	if diff != "" {
		return "\n-got +want\n" + diff
	}

	return ""
}
