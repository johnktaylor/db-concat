package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type integrationTestCase struct {
	name            string
	instructions    string
	output          string
	resultFile      string
	expected        string
	expectedText    string
	args            []string
	shouldFail      bool
	stdoutFile      string
	stderrFile      string
	expectedError   string
	preservedOutput string
}

// main builds the executable, runs all integration cases, reports their results, and exits nonzero when any fail.
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
	// Build the executable under test before launching isolated integration cases.
	buildCommand := exec.Command("go", "build", "-o", executablePath, ".")
	buildOutput, err := buildCommand.CombinedOutput()
	if err != nil {
		fmt.Printf("Build failed: %s\n%s", err, string(buildOutput))
		os.Exit(1)
	}

	tests := []integrationTestCase{
		{
			name:         "Parameter Files (--param-file)",
			instructions: "tests/instructions_param_file.dsl",
			output:       "tests/output_param_file.sql",
			expected:     "tests/expected_output_param_file.sql",
			args:         []string{"--param-file", "tests/params.txt"},
		},
		{
			name:         "DSL param overrides Parameter File",
			instructions: "tests/instructions_param_overrides_file.dsl",
			output:       "tests/output_param_overrides_file.sql",
			expectedText: "dsl",
			args:         []string{"--param-file", "tests/params_param_override.txt"},
		},
		{
			name:         "Command-line Parameters (--param)",
			instructions: "tests/instructions_cli_param.dsl",
			output:       "tests/output_cli_param.sql",
			expected:     "tests/expected_output_cli_param.sql",
			args:         []string{"--param", "CLI_VAR=1"},
		},
		{
			name:          "Invalid command-line parameter",
			instructions:  "tests/instructions_output.dsl",
			shouldFail:    true,
			expectedError: "Invalid --param value",
			args:          []string{"--param", "INVALID"},
		},
		{
			name:          "Invalid empty DSL param key",
			instructions:  "tests/instructions_invalid_param_empty_key.dsl",
			shouldFail:    true,
			expectedError: "invalid param command format",
		},
		{
			name:          "Invalid empty DSL set key",
			instructions:  "tests/instructions_invalid_set_empty_key.dsl",
			shouldFail:    true,
			expectedError: "invalid set command format",
		},
		{
			name:          "Invalid empty parameter-file key",
			instructions:  "tests/instructions_output.dsl",
			args:          []string{"--param-file", "tests/params_invalid_empty_key.txt"},
			shouldFail:    true,
			expectedError: "invalid parameter file line format",
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
			name:         "Command-line output overrides DSL output",
			instructions: "tests/instructions_output_precedence.dsl",
			resultFile:   "tests/output_cli_output_precedence.sql",
			expected:     "tests/expected_output_file.sql",
			args:         []string{"--output", "tests/output_cli_output_precedence.sql"},
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
			name:            "Failed concatenation preserves existing output",
			instructions:    "tests/instructions_output_failure.dsl",
			output:          "tests/output_preserve_on_error.sql",
			shouldFail:      true,
			expectedError:   "error opening file",
			preservedOutput: "tests/expected_preserved_output.sql",
		},
		{
			name:          "Unclosed text block",
			instructions:  "tests/instructions_unclosed_text_block.dsl",
			output:        "tests/output_error_unclosed_text_block.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_unclosed_text_block.txt",
			expectedError: "unclosed text block",
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
			name:         "Concat preserves literal @@ escapes in filenames",
			instructions: "tests/instructions_concat_literal_atat.dsl",
			output:       "tests/output_concat_literal_atat.sql",
			expected:     "tests/expected_output_concat_literal_atat.sql",
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
		{
			name:         "Inactive branch does not change prefix",
			instructions: "tests/instructions_inactive_prefix.dsl",
			output:       "tests/output_inactive_prefix.sql",
			expectedText: "SELECT 1;",
		},
		{
			name:         "Processing-time substitution",
			instructions: "tests/instructions_processing_time_substitution.dsl",
			output:       "tests/output_processing_time_substitution.sql",
			expected:     "tests/expected_output_processing_time_substitution.sql",
		},
		{
			name:         "Include path parameter substitution",
			instructions: "tests/instructions_include_substitution.dsl",
			output:       "tests/output_include_substitution.sql",
			expected:     "tests/expected_output_include_substitution.sql",
		},
		{
			name:          "Print missing parameter",
			instructions:  "tests/instructions_print_missing.dsl",
			output:        "tests/output_print_missing.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_print_missing.txt",
			expectedError: "parameter not found",
		},
		{
			name:         "Relative DSL output path resolution",
			instructions: "tests/relative_output/instructions_relative_output.dsl",
			resultFile:   "tests/relative_output/generated/out.sql",
			expected:     "tests/expected_output_relative_output.sql",
		},
		{
			name:         "Prefixed text-end is required",
			instructions: "tests/instructions_prefix_text_block.dsl",
			output:       "tests/output_prefix_text_block.sql",
			expected:     "tests/expected_output_prefix_text_block.sql",
		},
		{
			name:          "Invalid output command format",
			instructions:  "tests/instructions_invalid_output.dsl",
			output:        "tests/output_invalid_output.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_invalid_output.txt",
			expectedError: "invalid output command format",
		},
		{
			name:          "Invalid concat command format",
			instructions:  "tests/instructions_invalid_concat.dsl",
			output:        "tests/output_invalid_concat.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_invalid_concat.txt",
			expectedError: "invalid concat command format",
		},
		{
			name:          "Invalid include command format",
			instructions:  "tests/instructions_invalid_include.dsl",
			output:        "tests/output_invalid_include.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_invalid_include.txt",
			expectedError: "invalid include command format",
		},
		{
			name:          "Invalid print command format",
			instructions:  "tests/instructions_invalid_print.dsl",
			output:        "tests/output_invalid_print.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_invalid_print.txt",
			expectedError: "invalid print command format",
		},
		{
			name:          "Invalid if command format",
			instructions:  "tests/instructions_invalid_if.dsl",
			output:        "tests/output_invalid_if.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_invalid_if.txt",
			expectedError: "invalid if command format",
		},
		{
			name:          "Invalid else command format",
			instructions:  "tests/instructions_invalid_else.dsl",
			output:        "tests/output_invalid_else.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_invalid_else.txt",
			expectedError: "invalid else command format",
		},
		{
			name:          "Duplicate else command",
			instructions:  "tests/instructions_duplicate_else.dsl",
			shouldFail:    true,
			expectedError: "duplicate else for if block",
		},
		{
			name:          "Include cycle",
			instructions:  "tests/instructions_include_cycle.dsl",
			shouldFail:    true,
			expectedError: "include cycle detected",
		},
		{
			name:          "Invalid endif command format",
			instructions:  "tests/instructions_invalid_endif.dsl",
			output:        "tests/output_invalid_endif.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_invalid_endif.txt",
			expectedError: "invalid endif command format",
		},
		{
			name:          "Invalid text-begin command format",
			instructions:  "tests/instructions_invalid_text_begin.dsl",
			output:        "tests/output_invalid_text_begin.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_invalid_text_begin.txt",
			expectedError: "invalid text-begin command format",
		},
		{
			name:          "Invalid set-prefix command format",
			instructions:  "tests/instructions_invalid_set_prefix.dsl",
			output:        "tests/output_invalid_set_prefix.sql",
			shouldFail:    true,
			stderrFile:    "tests/error_invalid_set_prefix.txt",
			expectedError: "invalid set-prefix command format",
		},
	}

	failedTests := 0
	// Run each case with separately captured output so failures identify the affected scenario.
	for _, testCase := range tests {
		fmt.Printf("\n--- Test: %s ---\n", testCase.name)

		var commandArguments []string
		if len(testCase.args) > 0 {
			commandArguments = append(commandArguments, testCase.args...)
		}
		if testCase.output != "" && testCase.stdoutFile == "" {
			commandArguments = append(commandArguments, "--output", testCase.output)
		}
		commandArguments = append(commandArguments, testCase.instructions)

		// Seed a destination file when a failure case must prove it remains unchanged.
		if testCase.preservedOutput != "" {
			preservedOutputContent, err := os.ReadFile(testCase.preservedOutput)
			if err != nil {
				fmt.Printf("Failed to read preserved output fixture: %s\n", err)
				failedTests++
				continue
			}
			if err := os.WriteFile(testCase.output, preservedOutputContent, 0644); err != nil {
				fmt.Printf("Failed to seed output file: %s\n", err)
				failedTests++
				continue
			}
		}

		testCommand := exec.Command(executablePath, commandArguments...)

		var stdout, stderr bytes.Buffer
		if testCase.stdoutFile != "" {
			stdoutCaptureFile, err := os.Create(testCase.stdoutFile)
			if err != nil {
				fmt.Printf("Failed to create stdout file: %s\n", err)
				failedTests++
				continue
			}
			defer stdoutCaptureFile.Close()
			testCommand.Stdout = stdoutCaptureFile
		} else {
			testCommand.Stdout = &stdout
		}

		if testCase.stderrFile != "" {
			stderrCaptureFile, err := os.Create(testCase.stderrFile)
			if err != nil {
				fmt.Printf("Failed to create stderr file: %s\n", err)
				failedTests++
				continue
			}
			defer stderrCaptureFile.Close()
			testCommand.Stderr = stderrCaptureFile
		} else {
			testCommand.Stderr = &stderr
		}

		err := testCommand.Run()

		if testCase.shouldFail {
			if err == nil {
				fmt.Println("Test FAILED: Expected error, but got none.")
				failedTests++
			} else {
				if testCase.expectedError != "" {
					var errorOutput []byte
					var readErr error
					if testCase.stderrFile != "" {
						errorOutput, readErr = os.ReadFile(testCase.stderrFile)
					} else {
						errorOutput = stderr.Bytes()
					}

					if readErr != nil {
						fmt.Printf("Test FAILED: could not read stderr: %v\n", readErr)
						failedTests++
					} else if !bytes.Contains(errorOutput, []byte(testCase.expectedError)) {
						fmt.Printf("Test FAILED: Expected error message '%s' not found in stderr.\n", testCase.expectedError)
						failedTests++
					} else {
						fmt.Println("Test PASSED. (Expected error occurred)")
					}
				} else {
					fmt.Println("Test PASSED. (Expected error occurred)")
				}
			}
			if testCase.preservedOutput != "" {
				if err := compareFiles(testCase.output, testCase.preservedOutput); err != nil {
					fmt.Printf("Test FAILED: existing output was changed: %s\n", err)
					failedTests++
				} else {
					fmt.Println("Test PASSED. (Existing output preserved)")
				}
			}
		} else {
			if err != nil {
				fmt.Printf("Test FAILED: %s\n%s\n", err, stderr.String())
				failedTests++
			} else {
				var outputFilePath string
				if testCase.stdoutFile != "" {
					outputFilePath = testCase.stdoutFile
				} else if testCase.resultFile != "" {
					outputFilePath = testCase.resultFile
				} else {
					outputFilePath = testCase.output
				}

				var comparisonError error
				if testCase.expectedText != "" {
					comparisonError = compareFileToText(outputFilePath, testCase.expectedText)
				} else {
					comparisonError = compareFiles(outputFilePath, testCase.expected)
				}
				if comparisonError != nil {
					fmt.Printf("Test FAILED: %s\n", comparisonError)
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

	// Return a failing process status when any integration case did not meet its expectation.
	if failedTests > 0 {
		os.Exit(1)
	}
}

// compareFileToText compares a file with exact expected text and returns an I/O or mismatch error.
func compareFileToText(filename, expectedText string) error {
	actualContent, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("error reading file %s: %v", filename, err)
	}
	if !bytes.Equal(actualContent, []byte(expectedText)) {
		return fmt.Errorf("output mismatch for %s", filename)
	}
	return nil
}

// compareFiles compares two files after normalizing carriage returns and returns an I/O or mismatch error.
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

// cleanup removes generated test outputs and reports glob errors without returning a value.
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
