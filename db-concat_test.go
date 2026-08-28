package main

import (
	"bytes"
	"errors"
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
func fixturePath(pathElements ...string) string {
	return filepath.Join(append([]string{"tests"}, pathElements...)...)
}

// runCommand invokes the in-process CLI with arguments and returns captured output and any error.
func runCommand(arguments ...string) commandResult {
	var standardOutput, standardError bytes.Buffer
	err := run(arguments, &standardOutput, &standardError)
	return commandResult{standardOutput: standardOutput.String(), standardError: standardError.String(), err: err}
}

// runFixtureToFile executes one instruction fixture with a temporary output file and returns its content and command result.
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
func normalizedContent(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r"), nil)
}

// requireFixtureOutput verifies successful output against an expected repository fixture.
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
	requireFixtureError(testingContext, "instructions_output.dsl", "Invalid --param value", "--param", "INVALID")
}

// TestWhitespaceOnlyCommandLineParameterKey verifies a blank CLI parameter key is rejected.
func TestWhitespaceOnlyCommandLineParameterKey(testingContext *testing.T) {
	requireFixtureError(testingContext, "instructions_output.dsl", "Invalid --param value", "--param", "   =value")
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
