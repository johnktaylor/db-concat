package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestIntegrationSuite runs the existing integration harness from the project root and fails when any case fails.
func TestIntegrationSuite(testingContext *testing.T) {
	testingContext.Helper()

	// Resolve from this source file so the test works regardless of the package's runtime working directory.
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		testingContext.Fatal("could not determine the integration test source location")
	}
	projectRoot := filepath.Dir(filepath.Dir(sourceFile))

	// Run the harness in the project root because its fixture paths are project-relative.
	testCommand := exec.Command("go", "run", "-buildvcs=false", "tests/run_tests.go")
	testCommand.Dir = projectRoot
	commandOutput, err := testCommand.CombinedOutput()
	if err != nil {
		testingContext.Fatalf("integration suite failed: %v\n%s", err, commandOutput)
	}
}
