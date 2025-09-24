package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type testCase struct {
	name          string
	instructions  string
	output        string
	expected      string
	args          []string
	shouldFail    bool
	stdoutFile    string
	stderrFile    string
	expectedError string
}

func main() {
	executableName := "db-concat"
	if runtime.GOOS == "windows" {
		executableName = "db-concat.exe"
	}
	executablePath := ".\\" + executableName
	if runtime.GOOS != "windows" {
		executablePath = "./" + executableName
	}

	fmt.Println("Building db-concat...")
	buildCmd := exec.Command("go", "build", "-o", executablePath, ".")
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Build failed: %s\n%s", err, string(buildOutput))
		os.Exit(1)
	}

	tests := []testCase{
		{
			name:         "Parameter Files (--param-file)",
			instructions: "tests/instructions_param_file.dsl",
			output:       "tests/output_param_file.sql",
			expected:     "tests/expected_output_param_file.sql",
			args:         []string{"--param-file", "tests/params.txt"},
		},
		{
			name:         "Command-line Parameters (--param)",
			instructions: "tests/instructions_cli_param.dsl",
			output:       "tests/output_cli_param.sql",
			expected:     "tests/expected_output_cli_param.sql",
			args:         []string{"--param", "CLI_VAR=1"},
		},
		{
			name:         "DSL param command",
			instructions: "tests/instructions_dsl_param.dsl",
			output:       "tests/output_dsl_param.sql",
			expected:     "tests/expected_output_dsl_param.sql",
		},
		{
			name:         "Parameter Precedence (CLI > DSL > File)",
			instructions: "tests/instructions_precedence.dsl",
			output:       "tests/output_precedence.sql",
			expected:     "tests/expected_output_precedence.sql",
			args:         []string{"--param-file", "tests/params_precedence.txt", "--param", "OVERRIDE_VAR=1"},
		},
		{
			name:         "if condition is true",
			instructions: "tests/instructions_if_true.dsl",
			output:       "tests/output_if_true.sql",
			expected:     "tests/expected_output_if_true.sql",
		},
		{
			name:         "if condition is false",
			instructions: "tests/instructions_if_false.dsl",
			output:       "tests/output_if_false.sql",
			expected:     "tests/expected_output_if_false.sql",
		},
		{
			name:         "print command",
			instructions: "tests/instructions_print.dsl",
			output:       "tests/output_print.sql",
			expected:     "tests/expected_output_print.sql",
		},
		{
			name:         "Output to stdout",
			instructions: "tests/instructions_output.dsl",
			stdoutFile:   "tests/output_stdout.txt",
			expected:     "tests/expected_output_stdout.txt",
		},
		{
			name:         "Output to file using --output flag",
			instructions: "tests/instructions_output.dsl",
			output:       "tests/output_file.sql",
			expected:     "tests/expected_output_file.sql",
			args:         []string{"--output", "tests/output_file.sql"},
		},
		{
			name:          "Unclosed if block",
			instructions:  "tests/instructions_unclosed_if.dsl",
			output:        "tests/output_error_unclosed_if.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_unclosed_if.txt",
			expectedError: "unclosed if block(s)",
		},
		{
			name:          "Unknown command",
			instructions:  "tests/instructions_unknown_command.dsl",
			output:        "tests/output_error_unknown_command.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_unknown_command.txt",
			expectedError: "unknown command",
		},
		{
			name:         "set command",
			instructions: "tests/instructions_set.dsl",
			output:       "tests/output_set.sql",
			expected:     "tests/expected_output_set.sql",
		},
		{
			name:         "Parameter Precedence (set > param)",
			instructions: "tests/instructions_set_vs_param.dsl",
			output:       "tests/output_set_vs_param.sql",
			expected:     "tests/expected_output_set_vs_param.sql",
		},
		{
			name:         "Parameter Precedence (CLI > set)",
			instructions: "tests/instructions_cli_vs_set.dsl",
			output:       "tests/output_cli_vs_set.sql",
			expected:     "tests/expected_output_cli_vs_set.sql",
			args:         []string{"--param", "PRECEDENCE_VAR=from_cli"},
		},
		{
			name:         "emit command",
			instructions: "tests/instructions_emit.dsl",
			output:       "tests/output_emit.sql",
			expected:     "tests/expected_output_emit.sql",
		},
		{
			name:         "Prefix commands (set-prefix, clear-prefix)",
			instructions: "tests/instructions_prefix.dsl",
			output:       "tests/output_prefix.sql",
			expected:     "tests/expected_output_prefix.sql",
		},
		{
			name:         "Nested if statements",
			instructions: "tests/instructions_nested_if.dsl",
			output:       "tests/output_nested_if.sql",
			expected:     "tests/expected_output_nested_if.sql",
		},
		{
			name:         "Numerical if Conditions",
			instructions: "tests/instructions_numerical_if.dsl",
			output:       "tests/output_numerical_if.sql",
			expected:     "tests/expected_output_numerical_if.sql",
		},
	}

	failedTests := 0
	for _, tc := range tests {
		fmt.Printf("\n--- Test: %s ---\n", tc.name)

		var cmdArgs []string
		if len(tc.args) > 0 {
			cmdArgs = append(cmdArgs, tc.args...)
		}
		if tc.output != "" && tc.stdoutFile == "" {
			cmdArgs = append(cmdArgs, "--output", tc.output)
		}
		cmdArgs = append(cmdArgs, tc.instructions)

		cmd := exec.Command(executablePath, cmdArgs...)

		var stdout, stderr bytes.Buffer
		if tc.stdoutFile != "" {
			outfile, err := os.Create(tc.stdoutFile)
			if err != nil {
				fmt.Printf("Failed to create stdout file: %s\n", err)
				failedTests++
				continue
			}
			defer outfile.Close()
			cmd.Stdout = outfile
		} else {
			cmd.Stdout = &stdout
		}

		if tc.stderrFile != "" {
			errfile, err := os.Create(tc.stderrFile)
			if err != nil {
				fmt.Printf("Failed to create stderr file: %s\n", err)
				failedTests++
				continue
			}
			defer errfile.Close()
			cmd.Stderr = errfile
		} else {
			cmd.Stderr = &stderr
		}

		err := cmd.Run()

		if tc.shouldFail {
			if err == nil {
				fmt.Println("Test FAILED: Expected error, but got none.")
				failedTests++
			} else {
				if tc.expectedError != "" {
					var errorOutput []byte
					var readErr error
					if tc.stderrFile != "" {
						errorOutput, readErr = os.ReadFile(tc.stderrFile)
					} else {
						errorOutput = stderr.Bytes()
					}

					if readErr != nil {
						fmt.Printf("Test FAILED: could not read stderr: %v\n", readErr)
						failedTests++
					} else if !bytes.Contains(errorOutput, []byte(tc.expectedError)) {
						fmt.Printf("Test FAILED: Expected error message '%s' not found in stderr.\n", tc.expectedError)
						failedTests++
					} else {
						fmt.Println("Test PASSED. (Expected error occurred)")
					}
				} else {
					fmt.Println("Test PASSED. (Expected error occurred)")
				}
			}
		} else {
			if err != nil {
				fmt.Printf("Test FAILED: %s\n%s\n", err, stderr.String())
				failedTests++
			} else {
				var outputFilePath string
				if tc.stdoutFile != "" {
					outputFilePath = tc.stdoutFile
				} else {
					outputFilePath = tc.output
				}

				if err := compareFiles(outputFilePath, tc.expected); err != nil {
					fmt.Printf("Test FAILED: %s\n", err)
					failedTests++
				} else {
					fmt.Println("Test PASSED.")
				}
			}
		}
	}

	fmt.Println("\n--- Test Summary ---")
	fmt.Printf("Total tests: %d\n", len(tests))
	fmt.Printf("Failed tests: %d\n", failedTests)

	fmt.Println("\nCleaning up generated test output files...")
	// cleanup()

	if failedTests > 0 {
		os.Exit(1)
	}
}

func compareFiles(file1, file2 string) error {
	// Read both files and normalize line endings by removing carriage returns.
	content1, err := os.ReadFile(file1)
	if err != nil {
		return fmt.Errorf("error reading file %s: %v", file1, err)
	}
	normalized1 := bytes.ReplaceAll(content1, []byte("\r"), []byte(""))

	content2, err := os.ReadFile(file2)
	if err != nil {
		return fmt.Errorf("error reading file %s: %v", file2, err)
	}
	normalized2 := bytes.ReplaceAll(content2, []byte("\r"), []byte(""))

	if !bytes.Equal(normalized1, normalized2) {
		return fmt.Errorf("output mismatch between %s and %s", file1, file2)
	}
	return nil
}

func cleanup() {
	files, err := filepath.Glob("tests/output_*")
	if err != nil {
		fmt.Printf("Error finding files to clean up: %v\n", err)
	}
	errorFiles, err := filepath.Glob("tests/error_*")
	if err != nil {
		fmt.Printf("Error finding files to clean up: %v\n", err)
	}
	files = append(files, errorFiles...)
	for _, file := range files {
		os.Remove(file)
	}
}
