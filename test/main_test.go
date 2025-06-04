package guvnor_test

import (
	"testing"
)

func TestFrameworkSetup(t *testing.T) {
	// This test confirms that files in the /test directory are picked up
	// by the test runner (make test -> go test -v ./...)
	if true != true {
		t.Error("Basic boolean logic failed, test setup is awry.")
	}
}
