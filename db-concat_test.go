package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type commandResult struct {
	standardOutput string
	standardError  string
	err            error
}

// fixturePath returns a repository-relative test fixture path from its path elements.
// Inputs: pathElements variadic slice of path segment strings.
// Outputs/Side effects: returns a joined relative path string rooted in tests/.
func fixturePath(pathElements ...string) string {
	return filepath.Join(append([]string{"tests"}, pathElements...)...)
}

// runCommand invokes the in-process CLI with arguments and returns captured output and any error.
// Inputs: arguments variadic slice of command-line argument strings.
// Outputs/Side effects: executes run and returns a commandResult containing standard output, standard error, and any error.
func runCommand(arguments ...string) commandResult {
	var standardOutput, standardError bytes.Buffer
	err := run(arguments, &standardOutput, &standardError)
	return commandResult{standardOutput: standardOutput.String(), standardError: standardError.String(), err: err}
}

// runFixtureToFile executes one instruction fixture with a temporary output file and returns its content and command result.
// Inputs: testingContext provides test execution controls and failure reporting, instructions fixture path, and optional CLI arguments.
// Outputs/Side effects: writes output to a temporary file, returns file bytes and commandResult, or fails testingContext on read error.
func runFixtureToFile(testingContext *testing.T, instructions string, arguments ...string) ([]byte, commandResult) {
	testingContext.Helper()
	outputPath := filepath.Join(testingContext.TempDir(), "output.sql")
	commandArguments := append([]string{}, arguments...)
	commandArguments = append(commandArguments, "--output", outputPath, fixturePath(instructions))
	result := runCommand(commandArguments...)
	if result.err != nil {
		return nil, result
	}
	output, err := os.ReadFile(outputPath)
	if err != nil {
		testingContext.Fatalf("read generated output: %v", err)
	}
	return output, result
}

// normalizedContent removes carriage returns so fixtures compare consistently across operating systems.
// Inputs: content byte slice containing line breaks.
// Outputs/Side effects: returns a new byte slice with \r bytes stripped.
func normalizedContent(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r"), nil)
}

// requireFixtureOutput verifies successful output against an expected repository fixture.
// Inputs: testingContext provides test execution controls and failure reporting, instructions fixture path, expected fixture path, and optional CLI arguments.
// Outputs/Side effects: executes the CLI and fails testingContext if execution fails or output does not match expected fixture.
func requireFixtureOutput(testingContext *testing.T, instructions, expected string, arguments ...string) {
	testingContext.Helper()
	actualContent, result := runFixtureToFile(testingContext, instructions, arguments...)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v; stderr: %s", result.err, result.standardError)
	}
	expectedContent, err := os.ReadFile(fixturePath(expected))
	if err != nil {
		testingContext.Fatalf("read expected fixture: %v", err)
	}
	if !bytes.Equal(normalizedContent(actualContent), normalizedContent(expectedContent)) {
		testingContext.Fatalf("output = %q, want %q", actualContent, expectedContent)
	}
}

// requireTextOutput verifies successful output against exact text.
// Inputs: testingContext provides test execution controls and failure reporting, instructions fixture path, expected output string, and optional CLI arguments.
// Outputs/Side effects: executes the CLI and fails testingContext if execution fails or output does not match expected string.
func requireTextOutput(testingContext *testing.T, instructions, expected string, arguments ...string) {
	testingContext.Helper()
	actualContent, result := runFixtureToFile(testingContext, instructions, arguments...)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v; stderr: %s", result.err, result.standardError)
	}
	if string(actualContent) != expected {
		testingContext.Fatalf("output = %q, want %q", actualContent, expected)
	}
}

// requireFixtureError verifies that processing a fixture fails with the expected message.
// Inputs: testingContext provides test execution controls and failure reporting, instructions fixture path, expectedError substring, and optional CLI arguments.
// Outputs/Side effects: executes the CLI and fails testingContext if execution succeeds or error does not contain expectedError.
func requireFixtureError(testingContext *testing.T, instructions, expectedError string, arguments ...string) {
	testingContext.Helper()
	commandArguments := append([]string{}, arguments...)
	commandArguments = append(commandArguments, fixturePath(instructions))
	result := runCommand(commandArguments...)
	if result.err == nil || !strings.Contains(result.err.Error(), expectedError) {
		testingContext.Fatalf("error = %v, want containing %q", result.err, expectedError)
	}
}

// TestParameterFiles verifies parameter-file values substitute into DSL commands.
func TestParameterFiles(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_param_file.dsl", "expected_output_param_file.sql", "--param-file", fixturePath("params.txt"))
}

// TestDSLParamOverridesParameterFile verifies DSL param values override parameter-file defaults.
func TestDSLParamOverridesParameterFile(testingContext *testing.T) {
	requireTextOutput(testingContext, "instructions_param_overrides_file.dsl", "dsl", "--param-file", fixturePath("params_param_override.txt"))
}

// TestCommandLineParameter verifies CLI parameters substitute into DSL commands.
func TestCommandLineParameter(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_cli_param.dsl", "expected_output_cli_param.sql", "--param", "CLI_VAR=1")
}

// TestInvalidCommandLineParameter verifies a CLI parameter without an equals sign is rejected.
func TestInvalidCommandLineParameter(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_output.dsl", "invalid --param value", "--param", "INVALID")
}

// TestWhitespaceOnlyCommandLineParameterKey verifies a blank CLI parameter key is rejected.
func TestWhitespaceOnlyCommandLineParameterKey(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_output.dsl", "invalid --param value", "--param", "   =value")
}

// TestParameterSubstitutionIsNonRecursive verifies replacement values are not scanned for more placeholders.
func TestParameterSubstitutionIsNonRecursive(testingContext *testing.T) {
	requireTextOutput(testingContext, "instructions_non_recursive_substitution.dsl", "${SECOND}", "--param", "FIRST=${SECOND}", "--param", "SECOND=expanded")
}

// TestEmptyDSLParamKey verifies a DSL param assignment requires a key.
func TestEmptyDSLParamKey(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_param_empty_key.dsl", "invalid param command format")
}

// TestEmptyDSLSetKey verifies a DSL set assignment requires a key.
func TestEmptyDSLSetKey(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_set_empty_key.dsl", "invalid set command format")
}

// TestEmptyParameterFileKey verifies parameter-file assignments require keys.
func TestEmptyParameterFileKey(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_output.dsl", "invalid parameter file line format", "--param-file", fixturePath("params_invalid_empty_key.txt"))
}

// TestDSLParamCommand verifies a DSL param can select a concat source.
func TestDSLParamCommand(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_dsl_param.dsl", "expected_output_dsl_param.sql")
}

// TestParameterPrecedence verifies CLI values override DSL and parameter-file values.
func TestParameterPrecedence(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_precedence.dsl", "expected_output_precedence.sql", "--param-file", fixturePath("params_precedence.txt"), "--param", "OVERRIDE_VAR=1")
}

// TestTrueConditional verifies a true if branch is selected.
func TestTrueConditional(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_if_true.dsl", "expected_output_if_true.sql")
}

// TestFalseConditional verifies a false if selects its else branch.
func TestFalseConditional(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_if_false.dsl", "expected_output_if_false.sql")
}

// TestPrintCommand verifies print emits the current parameter value.
func TestPrintCommand(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_print.dsl", "expected_output_print.sql")
}

// TestOutputToStandardOutput verifies omitting an output path writes only content to stdout.
func TestOutputToStandardOutput(testingContext *testing.T) {
	result := runCommand(fixturePath("instructions_output.dsl"))
	if result.err != nil {
		testingContext.Fatalf("run failed: %v", result.err)
	}
	expected, err := os.ReadFile(fixturePath("expected_output_stdout.txt"))
	if err != nil {
		testingContext.Fatalf("read expected fixture: %v", err)
	}
	if result.standardOutput != string(expected) {
		testingContext.Fatalf("stdout = %q, want %q", result.standardOutput, expected)
	}
}

// TestOutputFile verifies CLI output writes the expected content and default permissions.
func TestOutputFile(testingContext *testing.T) {
	outputPath := filepath.Join(testingContext.TempDir(), "output.sql")
	result := runCommand("--output", outputPath, fixturePath("instructions_output.dsl"))
	if result.err != nil {
		testingContext.Fatalf("run failed: %v", result.err)
	}
	actual, err := os.ReadFile(outputPath)
	if err != nil {
		testingContext.Fatalf("read output: %v", err)
	}
	expected, err := os.ReadFile(fixturePath("expected_output_file.sql"))
	if err != nil {
		testingContext.Fatalf("read expected fixture: %v", err)
	}
	if !bytes.Equal(normalizedContent(actual), normalizedContent(expected)) {
		testingContext.Fatalf("output = %q, want %q", actual, expected)
	}
	if runtime.GOOS != "windows" {
		fileInfo, err := os.Stat(outputPath)
		if err != nil {
			testingContext.Fatalf("inspect output permissions: %v", err)
		}
		if fileInfo.Mode().Perm() != defaultOutputPermission {
			testingContext.Fatalf("permissions = %04o, want %04o", fileInfo.Mode().Perm(), defaultOutputPermission)
		}
	}
}

// TestCommandLineOutputPrecedence verifies CLI output overrides a DSL output command.
func TestCommandLineOutputPrecedence(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_output_precedence.dsl", "expected_output_file.sql")
}

// TestUnclosedIfBlock verifies unmatched if blocks are rejected.
func TestUnclosedIfBlock(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_unclosed_if.dsl", "unclosed if block(s)")
}

// TestFailedConcatenationPreservesOutput verifies a failed source read does not replace an existing output.
func TestFailedConcatenationPreservesOutput(testingContext *testing.T) {
	outputPath := filepath.Join(testingContext.TempDir(), "preserved.sql")
	preservedContent, err := os.ReadFile(fixturePath("expected_preserved_output.sql"))
	if err != nil {
		testingContext.Fatalf("read preserved-output fixture: %v", err)
	}
	if err := os.WriteFile(outputPath, preservedContent, 0644); err != nil {
		testingContext.Fatalf("seed output: %v", err)
	}
	result := runCommand("--output", outputPath, fixturePath("instructions_output_failure.dsl"))
	if result.err == nil || !strings.Contains(result.err.Error(), "error opening file") {
		testingContext.Fatalf("error = %v, want source-open failure", result.err)
	}
	actualContent, err := os.ReadFile(outputPath)
	if err != nil {
		testingContext.Fatalf("read preserved output: %v", err)
	}
	if !bytes.Equal(actualContent, preservedContent) {
		testingContext.Fatalf("preserved output changed to %q", actualContent)
	}
}

// TestUnclosedTextBlock verifies unmatched text blocks are rejected.
func TestUnclosedTextBlock(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_unclosed_text_block.dsl", "unclosed text block")
}

// TestUnknownCommand verifies unrecognized DSL commands are rejected.
func TestUnknownCommand(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_unknown_command.dsl", "unknown command")
}

// TestSetCommand verifies set assigns substituted parameter values.
func TestSetCommand(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_set.dsl", "expected_output_set.sql")
}

// TestSetOverridesParam verifies set has higher precedence than DSL param.
func TestSetOverridesParam(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_set_vs_param.dsl", "expected_output_set_vs_param.sql")
}

// TestCommandLineOverridesSet verifies CLI parameters have higher precedence than set.
func TestCommandLineOverridesSet(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_cli_vs_set.dsl", "expected_output_cli_vs_set.sql", "--param", "PRECEDENCE_VAR=from_cli")
}

// TestEmitCommand verifies emitted text, substitution, and escape decoding.
func TestEmitCommand(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_emit.dsl", "expected_output_emit.sql")
}

// TestConcatPreservesLiteralEscapes verifies concat paths do not decode @@ sequences.
func TestConcatPreservesLiteralEscapes(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_concat_literal_atat.dsl", "expected_output_concat_literal_atat.sql")
}

// TestPrefixCommands verifies prefixed commands and clear-prefix behavior.
func TestPrefixCommands(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_prefix.dsl", "expected_output_prefix.sql")
}

// TestNestedConditionals verifies nested if and else branches.
func TestNestedConditionals(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_nested_if.dsl", "expected_output_nested_if.sql")
}

// TestNumericalConditions verifies numeric comparison operators.
func TestNumericalConditions(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_numerical_if.dsl", "expected_output_numerical_if.sql")
}

// TestInactiveBranchDoesNotChangePrefix verifies skipped commands cannot alter prefix state.
func TestInactiveBranchDoesNotChangePrefix(testingContext *testing.T) {
	requireTextOutput(testingContext, "instructions_inactive_prefix.dsl", "SELECT 1;")
}

// TestInactiveTextBlockDiscardsControlText verifies inactive block content cannot change control flow.
func TestInactiveTextBlockDiscardsControlText(testingContext *testing.T) {
	requireTextOutput(testingContext, "instructions_inactive_text_block.dsl", "active-else")
}

// TestProcessingTimeSubstitution verifies commands use parameter values present when processed.
func TestProcessingTimeSubstitution(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_processing_time_substitution.dsl", "expected_output_processing_time_substitution.sql")
}

// TestIncludePathSubstitution verifies parameters substitute into include paths.
func TestIncludePathSubstitution(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_include_substitution.dsl", "expected_output_include_substitution.sql")
}

// TestPrintMissingParameter verifies print fails for an undefined parameter.
func TestPrintMissingParameter(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_print_missing.dsl", "parameter not found")
}

// TestRelativeDSLOutputPath verifies DSL output paths resolve relative to the instruction file.
func TestRelativeDSLOutputPath(testingContext *testing.T) {
	workspace := testingContext.TempDir()
	instructionsDirectory := filepath.Join(workspace, "tests", "relative_output")
	if err := os.MkdirAll(filepath.Join(instructionsDirectory, "generated"), 0755); err != nil {
		testingContext.Fatalf("create fixture directories: %v", err)
	}
	sourceContent, err := os.ReadFile("1.sql")
	if err != nil {
		testingContext.Fatalf("read concat source fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "1.sql"), sourceContent, 0644); err != nil {
		testingContext.Fatalf("create concat source: %v", err)
	}
	instructionsPath := filepath.Join(instructionsDirectory, "instructions.dsl")
	instructions, err := os.ReadFile(fixturePath("relative_output", "instructions_relative_output.dsl"))
	if err != nil {
		testingContext.Fatalf("read relative-output instructions: %v", err)
	}
	if err := os.WriteFile(instructionsPath, instructions, 0644); err != nil {
		testingContext.Fatalf("create instructions: %v", err)
	}
	result := runCommand(instructionsPath)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v", result.err)
	}
	actual, err := os.ReadFile(filepath.Join(instructionsDirectory, "generated", "out.sql"))
	if err != nil {
		testingContext.Fatalf("read relative output: %v", err)
	}
	expected, err := os.ReadFile(fixturePath("expected_output_relative_output.sql"))
	if err != nil {
		testingContext.Fatalf("read expected relative output: %v", err)
	}
	if !bytes.Equal(normalizedContent(actual), normalizedContent(expected)) {
		testingContext.Fatalf("output = %q, want %q", actual, expected)
	}
}

// TestPrefixedTextEnd verifies an unprefixed text-end remains literal while a prefix is active.
func TestPrefixedTextEnd(testingContext *testing.T) {
	requireFixtureOutput(testingContext, "instructions_prefix_text_block.dsl", "expected_output_prefix_text_block.sql")
}

// TestInvalidOutputCommand verifies output requires a filename.
func TestInvalidOutputCommand(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_output.dsl", "invalid output command format")
}

// TestInvalidConcatCommand verifies concat requires a filename.
func TestInvalidConcatCommand(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_concat.dsl", "invalid concat command format")
}

// TestInvalidIncludeCommand verifies include requires a filename.
func TestInvalidIncludeCommand(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_include.dsl", "invalid include command format")
}

// TestInvalidPrintCommand verifies print requires a parameter name.
func TestInvalidPrintCommand(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_print.dsl", "invalid print command format")
}

// TestInvalidIfCommand verifies if requires a condition.
func TestInvalidIfCommand(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_if.dsl", "invalid if command format")
}

// TestInvalidElseCommand verifies else rejects arguments.
func TestInvalidElseCommand(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_else.dsl", "invalid else command format")
}

// TestDuplicateElse verifies each conditional permits at most one else.
func TestDuplicateElse(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_duplicate_else.dsl", "duplicate else for if block")
}

// TestIncludeCycle verifies recursive includes are rejected.
func TestIncludeCycle(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_include_cycle.dsl", "include cycle detected")
}

// TestInvalidEndIfCommand verifies endif rejects arguments.
func TestInvalidEndIfCommand(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_endif.dsl", "invalid endif command format")
}

// TestInvalidTextBeginCommand verifies text-begin rejects arguments.
func TestInvalidTextBeginCommand(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_text_begin.dsl", "invalid text-begin command format")
}

// TestInvalidSetPrefixCommand verifies set-prefix requires a prefix.
func TestInvalidSetPrefixCommand(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_invalid_set_prefix.dsl", "invalid set-prefix command format")
}

// TestHelp verifies help prints usage and succeeds without reading an instruction file.
func TestHelp(testingContext *testing.T) {
	result := runCommand("--help")
	if result.err != nil {
		testingContext.Fatalf("help returned an error: %v", result.err)
	}
	if !strings.Contains(result.standardError, "Usage of db-concat:") {
		testingContext.Fatalf("help output did not contain usage: %q", result.standardError)
	}
}

// TestInvalidFlag verifies an invalid flag is reported exactly once.
func TestInvalidFlag(testingContext *testing.T) {
	result := runCommand("--not-a-real-flag")
	if !errors.Is(result.err, errCommandLineAlreadyReported) {
		testingContext.Fatalf("error = %v, want reported command-line error", result.err)
	}
	if occurrences := strings.Count(result.standardError, "flag provided but not defined"); occurrences != 1 {
		testingContext.Fatalf("flag error occurred %d times, want 1: %q", occurrences, result.standardError)
	}
}

// TestParamFileWhitespaceAndEmptyEntries verifies that comma-separated param-file entries with whitespace and empty items are handled cleanly.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if CLI execution fails or whitespace/empty parameter file entries are not handled cleanly.
func TestParamFileWhitespaceAndEmptyEntries(testingContext *testing.T) {
	tempDir := testingContext.TempDir()
	paramFile1 := filepath.Join(tempDir, "p1.txt")
	paramFile2 := filepath.Join(tempDir, "p2.txt")
	if err := os.WriteFile(paramFile1, []byte("VAR1=alpha\n"), 0644); err != nil {
		testingContext.Fatalf("write p1: %v", err)
	}
	if err := os.WriteFile(paramFile2, []byte("VAR2=beta\n"), 0644); err != nil {
		testingContext.Fatalf("write p2: %v", err)
	}
	dslFile := filepath.Join(tempDir, "test.dsl")
	dslContent := "emit ${VAR1}_${VAR2}\n"
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}
	flagValue := fmt.Sprintf("  %s  , ,  %s  ", paramFile1, paramFile2)
	result := runCommand("--param-file", flagValue, dslFile)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", result.err, result.standardError)
	}
	if result.standardOutput != "alpha_beta" {
		testingContext.Fatalf("stdout = %q, want alpha_beta", result.standardOutput)
	}
}

// TestStdoutBufferingOnFailure verifies that stdout mode buffers output and produces nothing on standard output if concatenation fails mid-run.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if mid-run failure does not occur or standard output is not completely buffered.
func TestStdoutBufferingOnFailure(testingContext *testing.T) {
	result := runCommand(fixturePath("instructions_output_failure.dsl"))
	if result.err == nil {
		testingContext.Fatalf("expected error on missing concat file, got nil")
	}
	if result.standardOutput != "" {
		testingContext.Fatalf("standard output was not buffered; got %q, want empty", result.standardOutput)
	}
}

// TestLiteralDoubleAtEscape verifies that @@@@ escapes to literal @@ in emit, print, and text-begin blocks.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if execution fails or @@@@ sequences are not escaped to literal @@.
func TestLiteralDoubleAtEscape(testingContext *testing.T) {
	tempDir := testingContext.TempDir()
	dslFile := filepath.Join(tempDir, "escape.dsl")
	dslContent := "emit SELECT @@@@spid;@@n\nemit @@@@n@@n\ntext-begin\nSELECT @@@@rowcount;\ntext-end\n"
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}
	result := runCommand(dslFile)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", result.err, result.standardError)
	}
	expected := "SELECT @@spid;\n@@n\nSELECT @@rowcount;\n"
	if result.standardOutput != expected {
		testingContext.Fatalf("stdout = %q, want %q", result.standardOutput, expected)
	}
}

// TestEvaluateConditionWhitespaceAndEmptyKey verifies whitespace trimming around operators, != comparison, missing keys evaluating to false, and empty key error rejection.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if condition parsing, evaluation, or error rejection behaves incorrectly.
func TestEvaluateConditionWhitespaceAndEmptyKey(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	// Condition with spaces around = and !=, and missing key
	validDSL := filepath.Join(tempDir, "cond_valid.dsl")
	validContent := "param A=1\nif   A   =   1   \nemit equal\nendif\nif A != 2\nemit not-equal\nendif\nif MISSING_KEY=1\nemit should-not-emit\nendif\nif MISSING_KEY!=1\nemit should-not-emit-missing\nendif\n"
	if err := os.WriteFile(validDSL, []byte(validContent), 0644); err != nil {
		testingContext.Fatalf("write valid dsl: %v", err)
	}
	result := runCommand(validDSL)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", result.err, result.standardError)
	}
	expected := "equalnot-equal"
	if result.standardOutput != expected {
		testingContext.Fatalf("stdout = %q, want %q", result.standardOutput, expected)
	}

	// Condition with empty key
	invalidDSL := filepath.Join(tempDir, "cond_invalid.dsl")
	invalidContent := "if   =value\nemit bad\nendif\n"
	if err := os.WriteFile(invalidDSL, []byte(invalidContent), 0644); err != nil {
		testingContext.Fatalf("write invalid dsl: %v", err)
	}
	invalidResult := runCommand(invalidDSL)
	if invalidResult.err == nil || !strings.Contains(invalidResult.err.Error(), "invalid condition format") {
		testingContext.Fatalf("error = %v, want containing invalid condition format", invalidResult.err)
	}
}

// TestHandleOutputAndConcatEmptyArgs verifies that handleOutputCommand and handleConcatCommand return errors instead of panicking when arguments are empty.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if handleOutputCommand or handleConcatCommand fails to return expected formatting errors.
func TestHandleOutputAndConcatEmptyArgs(testingContext *testing.T) {
	var out string
	err := handleOutputCommand("", &out, ".", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid output command format") {
		testingContext.Fatalf("handleOutputCommand(\"\") error = %v, want invalid output command format", err)
	}
	err = handleOutputCommand("   ", &out, ".", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid output command format") {
		testingContext.Fatalf("handleOutputCommand(\"   \") error = %v, want invalid output command format", err)
	}

	var items []ConcatItem
	err = handleConcatCommand("", &items, ".", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid concat command format") {
		testingContext.Fatalf("handleConcatCommand(\"\") error = %v, want invalid concat command format", err)
	}
	err = handleConcatCommand("   ", &items, ".", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid concat command format") {
		testingContext.Fatalf("handleConcatCommand(\"   \") error = %v, want invalid concat command format", err)
	}
}

// TestDispatchCommandWhitespaceTrimming verifies that extra leading and trailing whitespace on command arguments is trimmed cleanly.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if execution fails or leading/trailing argument whitespace is not trimmed.
func TestDispatchCommandWhitespaceTrimming(testingContext *testing.T) {
	tempDir := testingContext.TempDir()
	sqlFile := filepath.Join(tempDir, "test.sql")
	if err := os.WriteFile(sqlFile, []byte("SELECT 1;"), 0644); err != nil {
		testingContext.Fatalf("write sql: %v", err)
	}
	dslFile := filepath.Join(tempDir, "whitespace.dsl")
	dslContent := fmt.Sprintf("param   MY_VAR=hello\nprint   MY_VAR   \nconcat   %s   \n", filepath.Base(sqlFile))
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}
	result := runCommand(dslFile)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", result.err, result.standardError)
	}
	if result.standardOutput != "helloSELECT 1;" {
		testingContext.Fatalf("stdout = %q, want helloSELECT 1;", result.standardOutput)
	}
}

// TestFileLineAndIncludeErrorContext verifies that errors report the instruction file and line number and wrap include chain context.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if error does not include expected file, line, and include chain context.
func TestFileLineAndIncludeErrorContext(testingContext *testing.T) {
	tempDir := testingContext.TempDir()
	childDSL := filepath.Join(tempDir, "child.dsl")
	// Line 1: comment, Line 2: unknown command
	childContent := "# comment\nunknown_cmd_here\n"
	if err := os.WriteFile(childDSL, []byte(childContent), 0644); err != nil {
		testingContext.Fatalf("write child dsl: %v", err)
	}

	parentDSL := filepath.Join(tempDir, "parent.dsl")
	// Line 1: comment, Line 2: include child.dsl
	parentContent := fmt.Sprintf("# parent\ninclude %s\n", filepath.Base(childDSL))
	if err := os.WriteFile(parentDSL, []byte(parentContent), 0644); err != nil {
		testingContext.Fatalf("write parent dsl: %v", err)
	}

	result := runCommand(parentDSL)
	if result.err == nil {
		testingContext.Fatalf("expected error, got nil")
	}
	errMsg := result.err.Error()
	if !strings.Contains(errMsg, "parent.dsl:2:") {
		testingContext.Fatalf("error %q should contain parent.dsl:2:", errMsg)
	}
	if !strings.Contains(errMsg, "child.dsl:2:") {
		testingContext.Fatalf("error %q should contain child.dsl:2:", errMsg)
	}
	if !strings.Contains(errMsg, "unknown command: unknown_cmd_here") {
		testingContext.Fatalf("error %q should contain unknown command", errMsg)
	}
}

// TestScannerErrorBeforeUnclosedBlock verifies that scanner errors inside a text block surface token-too-long with file and line rather than unclosed text block.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if token-too-long error is masked by unclosed text block or misses line context.
func TestScannerErrorBeforeUnclosedBlock(testingContext *testing.T) {
	tempDir := testingContext.TempDir()
	dslFile := filepath.Join(tempDir, "toolong.dsl")
	// Over 1 MiB line inside text-begin
	hugeLine := strings.Repeat("A", maximumInputLineBytes+10)
	dslContent := "text-begin\n" + hugeLine + "\ntext-end\n"
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}
	result := runCommand(dslFile)
	if result.err == nil {
		testingContext.Fatalf("expected error, got nil")
	}
	errMsg := result.err.Error()
	if strings.Contains(errMsg, "unclosed text block") {
		testingContext.Fatalf("error %q should not be masked as unclosed text block", errMsg)
	}
	if !strings.Contains(errMsg, "token too long") {
		testingContext.Fatalf("error %q should contain token too long", errMsg)
	}
	if !strings.Contains(errMsg, "toolong.dsl:2:") {
		testingContext.Fatalf("error %q should identify toolong.dsl:2:", errMsg)
	}
}

// TestWrappedErrorSentinel verifies that wrapped errors maintain the error chain so errors.Is works with sentinel errors like os.ErrNotExist.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if error chain fails errors.Is matching against os.ErrNotExist.
func TestWrappedErrorSentinel(testingContext *testing.T) {
	tempDir := testingContext.TempDir()
	dslFile := filepath.Join(tempDir, "missing_inc.dsl")
	dslContent := "include nonexistent_file_xyz.dsl\n"
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}
	result := runCommand(dslFile)
	if result.err == nil {
		testingContext.Fatalf("expected error, got nil")
	}
	if !errors.Is(result.err, os.ErrNotExist) {
		testingContext.Fatalf("errors.Is(err, os.ErrNotExist) = false for error: %v", result.err)
	}
}

// TestMultipleParamFilesOverlappingKeys verifies that when multiple parameter files define the same keys,
// later parameter files overwrite earlier definitions while retaining unique keys from earlier files.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if CLI execution fails or output does not reflect later file precedence.
func TestMultipleParamFilesOverlappingKeys(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	// Seed first parameter file with a shared key and a unique key.
	paramFile1 := filepath.Join(tempDir, "params1.txt")
	paramContent1 := "SHARED_KEY=first_file\nONLY_IN_ONE=unique_one\nOVERRIDE_ME=initial\n"
	if err := os.WriteFile(paramFile1, []byte(paramContent1), 0644); err != nil {
		testingContext.Fatalf("write params1: %v", err)
	}

	// Seed second parameter file with overlapping shared key and another unique key.
	paramFile2 := filepath.Join(tempDir, "params2.txt")
	paramContent2 := "SHARED_KEY=second_file\nONLY_IN_TWO=unique_two\n"
	if err := os.WriteFile(paramFile2, []byte(paramContent2), 0644); err != nil {
		testingContext.Fatalf("write params2: %v", err)
	}

	// Seed third parameter file overriding OVERRIDE_ME.
	paramFile3 := filepath.Join(tempDir, "params3.txt")
	paramContent3 := "OVERRIDE_ME=third_file\n"
	if err := os.WriteFile(paramFile3, []byte(paramContent3), 0644); err != nil {
		testingContext.Fatalf("write params3: %v", err)
	}

	// DSL references all keys.
	dslFile := filepath.Join(tempDir, "test.dsl")
	dslContent := "emit SHARED=${SHARED_KEY};ONE=${ONLY_IN_ONE};TWO=${ONLY_IN_TWO};OVERRIDE=${OVERRIDE_ME}\n"
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}

	// Pass comma-separated list of parameter files to verify later files win for overlapping keys.
	paramArg := fmt.Sprintf("%s,%s,%s", paramFile1, paramFile2, paramFile3)
	result := runCommand("--param-file", paramArg, dslFile)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", result.err, result.standardError)
	}

	expectedOutput := "SHARED=second_file;ONE=unique_one;TWO=unique_two;OVERRIDE=third_file"
	if result.standardOutput != expectedOutput {
		testingContext.Fatalf("stdout = %q, want %q", result.standardOutput, expectedOutput)
	}
}

// TestPrefixScopingAcrossIncludes verifies that an included file begins in an unprefixed state
// regardless of the parent's prefix, and that the parent file's active prefix is restored upon returning.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if execution fails or commands are not scoped correctly by prefix.
func TestPrefixScopingAcrossIncludes(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	// Child file starts unprefixed; it executes an unprefixed emit, then sets its own prefix and executes a prefixed emit.
	childDSL := filepath.Join(tempDir, "child.dsl")
	childContent := "emit child_unprefixed@@n\nset-prefix childpfx\nchildpfx:emit child_prefixed@@n\n"
	if err := os.WriteFile(childDSL, []byte(childContent), 0644); err != nil {
		testingContext.Fatalf("write child dsl: %v", err)
	}

	// Parent file sets parentpfx prefix, includes child, and verifies prefix restoration upon return.
	parentDSL := filepath.Join(tempDir, "parent.dsl")
	parentContent := "emit parent_start@@n\n" +
		"set-prefix parentpfx\n" +
		"parentpfx:emit parent_in_prefix@@n\n" +
		"emit parent_ignored_unprefixed@@n\n" +
		fmt.Sprintf("parentpfx:include %s\n", filepath.Base(childDSL)) +
		"emit parent_ignored_after_include@@n\n" +
		"parentpfx:emit parent_restored@@n\n" +
		"parentpfx:clear-prefix\n" +
		"emit parent_end@@n\n"
	if err := os.WriteFile(parentDSL, []byte(parentContent), 0644); err != nil {
		testingContext.Fatalf("write parent dsl: %v", err)
	}

	result := runCommand(parentDSL)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", result.err, result.standardError)
	}

	expectedOutput := "parent_start\nparent_in_prefix\nchild_unprefixed\nchild_prefixed\nparent_restored\nparent_end\n"
	if result.standardOutput != expectedOutput {
		testingContext.Fatalf("stdout = %q, want %q", result.standardOutput, expectedOutput)
	}
}

// TestChildToParentParameterPropagation verifies that parameter definitions and mutations made
// within an included DSL file via param and set are visible and retained in the parent file after inclusion.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if parameters defined in the child file are not available in the parent file.
func TestChildToParentParameterPropagation(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	// Child defines a new param, sets a new param, and updates an existing param using set.
	childDSL := filepath.Join(tempDir, "child.dsl")
	childContent := "param CHILD_VAR=from_child\nset CHILD_SET_VAR=from_child_set\nset PARENT_VAR=overridden_by_child\n"
	if err := os.WriteFile(childDSL, []byte(childContent), 0644); err != nil {
		testingContext.Fatalf("write child dsl: %v", err)
	}

	// Parent initializes PARENT_VAR, includes child, and accesses parameters after child completes.
	parentDSL := filepath.Join(tempDir, "parent.dsl")
	parentContent := "param PARENT_VAR=initial_parent_val\n" +
		fmt.Sprintf("include %s\n", filepath.Base(childDSL)) +
		"emit ${PARENT_VAR}@@n\n" +
		"emit ${CHILD_VAR}@@n\n" +
		"emit ${CHILD_SET_VAR}@@n\n" +
		"print CHILD_VAR\n" +
		"emit @@n\n" +
		"print CHILD_SET_VAR\n" +
		"emit @@n\n"
	if err := os.WriteFile(parentDSL, []byte(parentContent), 0644); err != nil {
		testingContext.Fatalf("write parent dsl: %v", err)
	}

	result := runCommand(parentDSL)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", result.err, result.standardError)
	}

	expectedOutput := "overridden_by_child\nfrom_child\nfrom_child_set\nfrom_child\nfrom_child_set\n"
	if result.standardOutput != expectedOutput {
		testingContext.Fatalf("stdout = %q, want %q", result.standardOutput, expectedOutput)
	}
}

// TestPrintCommandEscapeDecoding verifies that the print command decodes @@ escape sequences
// (such as @@n, @@t, @@s, and @@@@) in the printed parameter value.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if execution fails or escape sequences in the printed parameter are not decoded.
func TestPrintCommandEscapeDecoding(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	dslFile := filepath.Join(tempDir, "print_escapes.dsl")
	dslContent := "param ESCAPED_PARAM=Line1@@nLine2@@tTabbed@@sSpaced@@@@LiteralAt\nprint ESCAPED_PARAM\n"
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}

	result := runCommand(dslFile)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", result.err, result.standardError)
	}

	expectedOutput := "Line1\nLine2\tTabbed Spaced@@LiteralAt"
	if result.standardOutput != expectedOutput {
		testingContext.Fatalf("stdout = %q, want %q", result.standardOutput, expectedOutput)
	}
}

// TestIncludeMissingFileErrorContext verifies that including a nonexistent file fails with an error
// containing the parent file name, line number, include target name, and preserving os.ErrNotExist.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if the error is nil, missing line/file context, or fails errors.Is sentinel check.
func TestIncludeMissingFileErrorContext(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	parentDSL := filepath.Join(tempDir, "parent.dsl")
	// Line 1: comment
	// Line 2: comment
	// Line 3: include missing file
	parentContent := "# Header comment\n# Another comment\ninclude nonexistent_child_script.dsl\n"
	if err := os.WriteFile(parentDSL, []byte(parentContent), 0644); err != nil {
		testingContext.Fatalf("write parent dsl: %v", err)
	}

	result := runCommand(parentDSL)
	if result.err == nil {
		testingContext.Fatalf("expected error on missing include, got nil")
	}

	// Check sentinel error preservation.
	if !errors.Is(result.err, os.ErrNotExist) {
		testingContext.Fatalf("errors.Is(err, os.ErrNotExist) = false; error: %v", result.err)
	}

	errMsg := result.err.Error()
	// Check file:line context of the including file.
	if !strings.Contains(errMsg, "parent.dsl:3:") {
		testingContext.Fatalf("error %q should contain file and line parent.dsl:3:", errMsg)
	}
	// Check missing include filename in error message.
	if !strings.Contains(errMsg, "nonexistent_child_script.dsl") {
		testingContext.Fatalf("error %q should mention missing include file", errMsg)
	}
	if !strings.Contains(errMsg, "error in include") {
		testingContext.Fatalf("error %q should mention error in include", errMsg)
	}
}

// TestDiamondIncludeSuccess verifies that including the same file multiple times across different include
// branches or sequentially succeeds without falsely triggering include cycle detection.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if execution fails or diamond inclusion triggers an include cycle error.
func TestDiamondIncludeSuccess(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	// Leaf file included by multiple branches.
	leafDSL := filepath.Join(tempDir, "leaf.dsl")
	leafContent := "emit leaf@@n\n"
	if err := os.WriteFile(leafDSL, []byte(leafContent), 0644); err != nil {
		testingContext.Fatalf("write leaf: %v", err)
	}

	// Branch A includes leaf.
	branchA := filepath.Join(tempDir, "branch_a.dsl")
	branchAContent := fmt.Sprintf("emit branch_a_start@@n\ninclude %s\nemit branch_a_end@@n\n", filepath.Base(leafDSL))
	if err := os.WriteFile(branchA, []byte(branchAContent), 0644); err != nil {
		testingContext.Fatalf("write branch a: %v", err)
	}

	// Branch B includes leaf.
	branchB := filepath.Join(tempDir, "branch_b.dsl")
	branchBContent := fmt.Sprintf("emit branch_b_start@@n\ninclude %s\nemit branch_b_end@@n\n", filepath.Base(leafDSL))
	if err := os.WriteFile(branchB, []byte(branchBContent), 0644); err != nil {
		testingContext.Fatalf("write branch b: %v", err)
	}

	// Root includes branch A, branch B, and leaf again directly.
	rootDSL := filepath.Join(tempDir, "diamond_root.dsl")
	rootContent := fmt.Sprintf("emit root_start@@n\ninclude %s\ninclude %s\ninclude %s\nemit root_end@@n\n",
		filepath.Base(branchA), filepath.Base(branchB), filepath.Base(leafDSL))
	if err := os.WriteFile(rootDSL, []byte(rootContent), 0644); err != nil {
		testingContext.Fatalf("write root: %v", err)
	}

	result := runCommand(rootDSL)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", result.err, result.standardError)
	}

	expectedOutput := "root_start\nbranch_a_start\nleaf\nbranch_a_end\nbranch_b_start\nleaf\nbranch_b_end\nleaf\nroot_end\n"
	if result.standardOutput != expectedOutput {
		testingContext.Fatalf("stdout = %q, want %q", result.standardOutput, expectedOutput)
	}
}

// TestTopLevelLineExceedingLimit verifies that a line outside of a text block exceeding 1 MiB
// is rejected with a token-too-long scanner error and reports the file and line context.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if the oversized line is accepted or error does not report scanner failure.
func TestTopLevelLineExceedingLimit(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	dslFile := filepath.Join(tempDir, "huge_top_level.dsl")
	// Line 1: normal comment
	// Line 2: top-level line exceeding maximumInputLineBytes (1 MiB)
	hugeLine := strings.Repeat("X", maximumInputLineBytes+10)
	dslContent := "# Line 1\nemit " + hugeLine + "\n"
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}

	result := runCommand(dslFile)
	if result.err == nil {
		testingContext.Fatalf("expected error on >1 MiB line, got nil")
	}

	errMsg := result.err.Error()
	if !strings.Contains(errMsg, "token too long") {
		testingContext.Fatalf("error %q should contain token too long", errMsg)
	}
	if !strings.Contains(errMsg, "huge_top_level.dsl:2:") {
		testingContext.Fatalf("error %q should identify huge_top_level.dsl:2:", errMsg)
	}
}

// TestParamFileNonexistentError verifies that pointing --param-file to a nonexistent file
// returns an error that preserves os.ErrNotExist and reports the missing filename.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if the command succeeds or the returned error does not wrap os.ErrNotExist.
func TestParamFileNonexistentError(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	dslFile := filepath.Join(tempDir, "test.dsl")
	dslContent := "emit hello\n"
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}

	missingParamFile := filepath.Join(tempDir, "missing_params_file.txt")
	result := runCommand("--param-file", missingParamFile, dslFile)
	if result.err == nil {
		testingContext.Fatalf("expected error for nonexistent --param-file, got nil")
	}

	// Verify error wrapping preserves os.ErrNotExist.
	if !errors.Is(result.err, os.ErrNotExist) {
		testingContext.Fatalf("errors.Is(err, os.ErrNotExist) = false; error: %v", result.err)
	}

	errMsg := result.err.Error()
	if !strings.Contains(errMsg, "error loading parameters from file") {
		testingContext.Fatalf("error %q should contain 'error loading parameters from file'", errMsg)
	}
	if !strings.Contains(errMsg, "missing_params_file.txt") {
		testingContext.Fatalf("error %q should mention missing parameter filename", errMsg)
	}
}

// TestOutputNonexistentDirectoryError verifies that specifying an output file in a nonexistent
// directory via --output fails with an error preserving os.ErrNotExist.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if the command succeeds or does not report an error wrapping os.ErrNotExist.
func TestOutputNonexistentDirectoryError(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	dslFile := filepath.Join(tempDir, "test.dsl")
	dslContent := "emit hello\n"
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}

	nonexistentDirOutput := filepath.Join(tempDir, "nonexistent_dir", "out.sql")
	result := runCommand("--output", nonexistentDirOutput, dslFile)
	if result.err == nil {
		testingContext.Fatalf("expected error for nonexistent output directory, got nil")
	}

	// Verify error wrapping preserves os.ErrNotExist.
	if !errors.Is(result.err, os.ErrNotExist) {
		testingContext.Fatalf("errors.Is(err, os.ErrNotExist) = false; error: %v", result.err)
	}

	errMsg := result.err.Error()
	if !strings.Contains(errMsg, "error writing output") {
		testingContext.Fatalf("error %q should contain 'error writing output'", errMsg)
	}
	if !strings.Contains(errMsg, "error creating temporary output file") {
		testingContext.Fatalf("error %q should contain 'error creating temporary output file'", errMsg)
	}
}

// TestOverwriteExistingOutputPreservesPermissions verifies that replacing an existing output file succeeds,
// updates file contents, and preserves the pre-existing file permissions.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if the output file is not overwritten or original permissions are altered.
func TestOverwriteExistingOutputPreservesPermissions(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	dslFile := filepath.Join(tempDir, "test.dsl")
	dslContent := "emit new_replacement_content"
	if err := os.WriteFile(dslFile, []byte(dslContent), 0644); err != nil {
		testingContext.Fatalf("write dsl: %v", err)
	}

	outputPath := filepath.Join(tempDir, "existing_output.sql")
	initialContent := []byte("old_seeded_content")
	targetPermission := os.FileMode(0755)
	if err := os.WriteFile(outputPath, initialContent, targetPermission); err != nil {
		testingContext.Fatalf("seed output file: %v", err)
	}

	// Verify pre-existing permissions before run.
	initialStat, err := os.Stat(outputPath)
	if err != nil {
		testingContext.Fatalf("stat initial output file: %v", err)
	}
	expectedPerm := initialStat.Mode().Perm()

	result := runCommand("--output", outputPath, dslFile)
	if result.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", result.err, result.standardError)
	}

	// Check content was overwritten.
	actualContent, err := os.ReadFile(outputPath)
	if err != nil {
		testingContext.Fatalf("read overwritten output: %v", err)
	}
	if string(actualContent) != "new_replacement_content" {
		testingContext.Fatalf("output content = %q, want %q", string(actualContent), "new_replacement_content")
	}

	// Verify permissions are preserved on non-Windows systems.
	if runtime.GOOS != "windows" {
		newStat, err := os.Stat(outputPath)
		if err != nil {
			testingContext.Fatalf("stat overwritten output file: %v", err)
		}
		if newStat.Mode().Perm() != expectedPerm {
			testingContext.Fatalf("permissions = %04o, want preserved %04o", newStat.Mode().Perm(), expectedPerm)
		}
	}
}

// TestBareEmitEmitsEmptyString verifies that an emit command without arguments emits an empty string without error.
// Inputs: testingContext provides test execution controls and failure reporting.
// Outputs/Side effects: fails testingContext if execution fails or a bare emit command does not emit an empty string.
func TestBareEmitEmitsEmptyString(testingContext *testing.T) {
	tempDir := testingContext.TempDir()

	// Verify a DSL file containing solely a bare emit command produces empty output without error.
	soleBareEmitDSL := filepath.Join(tempDir, "sole_bare_emit.dsl")
	if err := os.WriteFile(soleBareEmitDSL, []byte("emit\n"), 0644); err != nil {
		testingContext.Fatalf("write sole bare emit dsl: %v", err)
	}

	soleResult := runCommand(soleBareEmitDSL)
	if soleResult.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", soleResult.err, soleResult.standardError)
	}
	if soleResult.standardOutput != "" {
		testingContext.Fatalf("stdout = %q, want empty string", soleResult.standardOutput)
	}

	// Verify bare emit commands interspersed with other emitted text produce no extra characters.
	interspersedDSL := filepath.Join(tempDir, "interspersed_bare_emit.dsl")
	interspersedContent := "emit\nemit hello\nemit\nemit  \nemit world\nemit\n"
	if err := os.WriteFile(interspersedDSL, []byte(interspersedContent), 0644); err != nil {
		testingContext.Fatalf("write interspersed dsl: %v", err)
	}

	interspersedResult := runCommand(interspersedDSL)
	if interspersedResult.err != nil {
		testingContext.Fatalf("run failed: %v, stderr: %s", interspersedResult.err, interspersedResult.standardError)
	}
	if interspersedResult.standardOutput != "helloworld" {
		testingContext.Fatalf("stdout = %q, want helloworld", interspersedResult.standardOutput)
	}
}
