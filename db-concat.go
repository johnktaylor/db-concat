package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maximumInputLineBytes   = 1024 * 1024
	defaultOutputPermission = os.FileMode(0644)
)

type textBlockMode int

const (
	textBlockNone textBlockMode = iota
	textBlockOutput
	textBlockDiscard
)

type ConcatItem struct {
	IsFile        bool
	ContentOrPath string
	BaseDir       string // Base directory used to resolve relative source-file paths.
}

type executionState struct {
	commandLineParameterNames map[string]bool
	dslSetParameterNames      map[string]bool
}

var errCommandLineAlreadyReported = errors.New("command-line error already reported")

// main runs the command-line application and exits nonzero after reporting an execution error.
// Inputs: reads command-line arguments from os.Args.
// Outputs/Side effects: writes output to os.Stdout or files, writes errors to os.Stderr, and exits process on failure.
func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, errCommandLineAlreadyReported) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

// run parses arguments, builds the concatenation plan, writes output, and returns validation, processing, or I/O errors.
// Inputs: arguments slice of strings, standardOutput writer, and standardError writer.
// Outputs: returns nil on success or an error if validation, processing, or writing fails.
func run(arguments []string, standardOutput io.Writer, standardError io.Writer) error {
	commandFlags := flag.NewFlagSet("db-concat", flag.ContinueOnError)
	commandFlags.SetOutput(standardError)
	parameterFileList := commandFlags.String("param-file", "", "Comma-separated list of parameter files (key=value per line)")
	var commandLineParameterValues stringList
	commandFlags.Var(&commandLineParameterValues, "param", "Key-value pair parameter (e.g., --param key=value). Can be specified multiple times.")
	commandLineOutputFile := commandFlags.String("output", "", "Output file path. If not specified, output goes to stdout.")
	if err := commandFlags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("%w: %w", errCommandLineAlreadyReported, err)
	}

	if commandFlags.NArg() != 1 {
		fmt.Fprintln(standardError, "Usage: db-concat [OPTIONS] <instructions_file>")
		commandFlags.PrintDefaults()
		return fmt.Errorf("expected exactly one instructions file")
	}

	instructionsFile := commandFlags.Arg(0)
	instructionsDir := filepath.Dir(instructionsFile)
	parameters := make(map[string]string)

	// Load parameters from files (lowest precedence)
	if *parameterFileList != "" {
		parameterFiles := strings.Split(*parameterFileList, ",")
		for _, parameterFile := range parameterFiles {
			parameterFile = strings.TrimSpace(parameterFile)
			if parameterFile == "" {
				continue
			}
			err := loadParamsFromFile(parameterFile, parameters)
			if err != nil {
				return fmt.Errorf("error loading parameters from file %s: %w", parameterFile, err)
			}
		}
	}

	// Load parameters from command line (highest precedence) before processing DSL instructions
	// Apply CLI values before DSL parsing so DSL commands cannot override them.
	state := executionState{commandLineParameterNames: make(map[string]bool), dslSetParameterNames: make(map[string]bool)}
	for _, parameterArgument := range commandLineParameterValues {
		parameterName, parameterValue, isValid := parseParameterAssignment(parameterArgument)
		if !isValid {
			return fmt.Errorf("invalid --param value %q: expected key=value with a non-empty key", parameterArgument)
		}
		parameters[parameterName] = parameterValue
		state.commandLineParameterNames[parameterName] = true
	}

	var dslOutputFile string
	var itemsToConcat []ConcatItem

	activeIncludes := make(map[string]bool)
	err := processInstructions(instructionsFile, &dslOutputFile, &itemsToConcat, parameters, instructionsDir, activeIncludes, &state)
	if err != nil {
		return fmt.Errorf("error processing instructions: %w", err)
	}

	finalOutputFile := dslOutputFile
	// An explicit CLI destination overrides the instruction-file default.
	if *commandLineOutputFile != "" {
		finalOutputFile = *commandLineOutputFile
	}

	// Materialize the complete plan without replacing a file destination until every item succeeds.
	err = writeOutput(finalOutputFile, itemsToConcat, standardOutput)
	if err != nil {
		return fmt.Errorf("error writing output: %w", err)
	}
	return nil
}

// writeOutput writes planned items to stdout or atomically replaces finalOutputFile after a successful file write.
// Inputs: finalOutputFile path string (empty for stdout), itemsToConcat slice of concatenation items, and standardOutput io.Writer.
// Outputs/Side effects: writes concatenated output to stdout or writes atomically to finalOutputFile, emits a success message to standardOutput on file write, and returns nil or an error.
func writeOutput(finalOutputFile string, itemsToConcat []ConcatItem, standardOutput io.Writer) error {
	// Stdout mode buffers all output so failures mid-concatenation do not emit partial content.
	if finalOutputFile == "" {
		var outputBuffer bytes.Buffer
		if err := runConcat(&outputBuffer, itemsToConcat); err != nil {
			return err
		}
		_, err := io.Copy(standardOutput, &outputBuffer)
		return err
	}

	// Preserve an existing destination's permissions, or use a predictable readable default for a new file.
	outputPermission := defaultOutputPermission
	existingOutputInfo, statErr := os.Stat(finalOutputFile)
	if statErr == nil {
		outputPermission = existingOutputInfo.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("error inspecting output file %s: %w", finalOutputFile, statErr)
	}

	// Create the temporary file beside the destination so a successful rename stays on the same filesystem.
	temporaryOutput, err := os.CreateTemp(filepath.Dir(finalOutputFile), "."+filepath.Base(finalOutputFile)+".tmp-*")
	if err != nil {
		return fmt.Errorf("error creating temporary output file for %s: %w", finalOutputFile, err)
	}
	temporaryOutputPath := temporaryOutput.Name()
	defer func() {
		temporaryOutput.Close()
		os.Remove(temporaryOutputPath)
	}()
	if err := temporaryOutput.Chmod(outputPermission); err != nil {
		return fmt.Errorf("error setting permissions on temporary output file %s: %w", temporaryOutputPath, err)
	}

	if err := runConcat(temporaryOutput, itemsToConcat); err != nil {
		return err
	}
	if err := temporaryOutput.Close(); err != nil {
		return fmt.Errorf("error closing temporary output file %s: %w", temporaryOutputPath, err)
	}
	if err := os.Rename(temporaryOutputPath, finalOutputFile); err != nil {
		return fmt.Errorf("error replacing output file %s: %w", finalOutputFile, err)
	}

	fmt.Fprintln(standardOutput, "Successfully concatenated files to output.")
	return nil
}

// loadParamsFromFile reads key=value entries from filename into parameters and returns parsing or I/O errors.
// Inputs: filename of parameter file, and parameters map to populate.
// Outputs: returns nil on success or an error if opening, reading, or parsing lines fails.
func loadParamsFromFile(filename string, parameters map[string]string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening parameter file %s: %w", filename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Permit generated parameter values while retaining a bounded per-line memory limit.
	scanner.Buffer(make([]byte, 64*1024), maximumInputLineBytes)
	// Skip comments and blank lines while preserving literal parameter values.
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parameterName, parameterValue, isValid := parseParameterAssignment(line)
		if isValid {
			parameters[parameterName] = parameterValue
		} else {
			return fmt.Errorf("invalid parameter file line format: %s", line)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading parameter file %s: %w", filename, err)
	}
	return nil
}

// parseParameterAssignment parses key=value text, trims the key, and reports whether the key is nonempty.
// Inputs: assignment string in key=value format.
// Outputs: parameterName string, parameterValue string, and isValid boolean indicating whether a non-empty key was found.
func parseParameterAssignment(assignment string) (parameterName, parameterValue string, isValid bool) {
	parameterParts := strings.SplitN(assignment, "=", 2)
	if len(parameterParts) != 2 {
		return "", "", false
	}
	parameterName = strings.TrimSpace(parameterParts[0])
	if parameterName == "" {
		return "", "", false
	}
	return parameterName, parameterParts[1], true
}

type stringList []string

// String returns the flag values as a comma-separated string for flag.Value display.
// Inputs: receiver stringList pointer.
// Outputs: comma-separated string representation of values.
func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

// Set appends one command-line flag value and always returns nil.
// Inputs: value string representing a CLI parameter flag.
// Outputs: always returns nil error.
func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

// substituteParams replaces known ${key} references in text using parameters and returns the substituted text.
// Inputs: text string containing potential ${key} placeholders, and parameters map.
// Outputs: substituted string with known placeholders replaced without recursive expansion.
func substituteParams(text string, parameters map[string]string) string {
	var substitutedText strings.Builder
	remainingText := text

	// Consume placeholders from the original text so inserted values are never scanned recursively.
	for {
		placeholderStart := strings.Index(remainingText, "${")
		if placeholderStart < 0 {
			substitutedText.WriteString(remainingText)
			break
		}
		placeholderEndOffset := strings.Index(remainingText[placeholderStart+2:], "}")
		if placeholderEndOffset < 0 {
			substitutedText.WriteString(remainingText)
			break
		}

		placeholderEnd := placeholderStart + 2 + placeholderEndOffset
		parameterName := remainingText[placeholderStart+2 : placeholderEnd]
		substitutedText.WriteString(remainingText[:placeholderStart])
		if parameterValue, exists := parameters[parameterName]; exists {
			substitutedText.WriteString(parameterValue)
		} else {
			substitutedText.WriteString(remainingText[placeholderStart : placeholderEnd+1])
		}
		remainingText = remainingText[placeholderEnd+1:]
	}
	return substitutedText.String()
}

// unescapeString converts DSL @@ escape sequences in text and returns the decoded text.
// It decodes @@@@ to literal @@ before interpreting other escape sequences (@@n, @@r, @@t, @@s).
// Inputs: text string containing potential DSL escape sequences.
// Outputs: string with escape sequences converted to their target characters.
func unescapeString(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))

	// Decode @@@@ before other escape substitutions so literal @@ sequences are preserved.
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "@@@@") {
			builder.WriteString("@@")
			i += 4
		} else if strings.HasPrefix(text[i:], "@@n") {
			builder.WriteByte('\n')
			i += 3
		} else if strings.HasPrefix(text[i:], "@@r") {
			builder.WriteByte('\r')
			i += 3
		} else if strings.HasPrefix(text[i:], "@@t") {
			builder.WriteByte('\t')
			i += 3
		} else if strings.HasPrefix(text[i:], "@@s") {
			builder.WriteByte(' ')
			i += 3
		} else {
			builder.WriteByte(text[i])
			i++
		}
	}
	return builder.String()
}

type conditionalFrame struct {
	active   bool
	elseSeen bool
	line     int
}

type conditionalStack []conditionalFrame

// push adds one conditional execution state and its source line number to stack.
// Inputs: isActive boolean indicating if branch is active, and lineNumber integer where condition began.
// Outputs/Side effects: appends frame to stack slice.
func (stack *conditionalStack) push(isActive bool, lineNumber int) {
	*stack = append(*stack, conditionalFrame{active: isActive, line: lineNumber})
}

// pop removes and returns the most recent conditional state, or returns an error when stack is empty.
// Inputs: receiver conditionalStack pointer.
// Outputs: removed conditionalFrame and nil error, or empty frame and an error if stack is empty.
func (stack *conditionalStack) pop() (conditionalFrame, error) {
	if len(*stack) == 0 {
		return conditionalFrame{}, fmt.Errorf("pop on empty stack")
	}
	activeFrame := (*stack)[len(*stack)-1]
	*stack = (*stack)[:len(*stack)-1]
	return activeFrame, nil
}

// peek returns the most recent conditional state without removing it, or an error when stack is empty.
// Inputs: receiver conditionalStack pointer.
// Outputs: top conditionalFrame and nil error, or empty frame and an error if stack is empty.
func (stack *conditionalStack) peek() (conditionalFrame, error) {
	if len(*stack) == 0 {
		return conditionalFrame{}, fmt.Errorf("peek on empty stack")
	}
	return (*stack)[len(*stack)-1], nil
}

// evaluateCondition evaluates a DSL comparison against parameters, trimming whitespace from key and value.
// Inputs: condition string (e.g. "key=val", "key!=val", or numerical comparisons) and parameters map.
// Outputs: boolean result of the condition and nil error, or false and an error if condition format is invalid or key is empty.
func evaluateCondition(condition string, parameters map[string]string) (bool, error) {
	operators := []string{">=", "<=", "!=", "=", ">", "<"}
	var operator, key, expectedValue string

	// Prefer multi-character operators so their leading character is not parsed first.
	for _, op := range operators {
		if strings.Contains(condition, op) {
			conditionParts := strings.SplitN(condition, op, 2)
			if len(conditionParts) == 2 {
				operator = op
				key = strings.TrimSpace(conditionParts[0])
				expectedValue = strings.TrimSpace(conditionParts[1])
				break
			}
		}
	}

	if operator == "" {
		return false, fmt.Errorf("invalid condition format: %s", condition)
	}

	if key == "" {
		return false, fmt.Errorf("invalid condition format: missing key in condition %q", condition)
	}

	actualValue, ok := parameters[key]
	if !ok {
		return false, nil // Key not found, condition is false
	}

	if operator == "=" {
		return actualValue == expectedValue, nil
	}
	if operator == "!=" {
		return actualValue != expectedValue, nil
	}

	// For numerical comparisons
	actualNumber, actualNumberErr := strconv.ParseFloat(actualValue, 64)
	expectedNumber, expectedNumberErr := strconv.ParseFloat(expectedValue, 64)

	if actualNumberErr != nil || expectedNumberErr != nil {
		return false, nil // One of the values is not a number, so comparison is false
	}

	switch operator {
	case ">":
		return actualNumber > expectedNumber, nil
	case ">=":
		return actualNumber >= expectedNumber, nil
	case "<":
		return actualNumber < expectedNumber, nil
	case "<=":
		return actualNumber <= expectedNumber, nil
	}

	return false, fmt.Errorf("unhandled operator: %s", operator)
}

// handleConditionalCommand updates conditionalStack and shouldSkip for an if, else, or endif command and returns validation errors.
// Inputs: command string ("if", "else", or "endif"), arguments string, parameters map, conditionalStack pointer, shouldSkip pointer, and lineNumber integer.
// Outputs: returns nil on success or an error if syntax, condition evaluation, or stack constraints are violated.
func handleConditionalCommand(command, arguments string, parameters map[string]string, conditionalStack *conditionalStack, shouldSkip *bool, lineNumber int) error {
	switch command {
	case "if":
		if strings.TrimSpace(arguments) == "" {
			return fmt.Errorf("invalid if command format: missing condition")
		}
		if *shouldSkip { // If already skipping, push false to stack and continue skipping
			conditionalStack.push(false, lineNumber)
			return nil
		}
		conditionTrue, err := evaluateCondition(arguments, parameters)
		if err != nil {
			return err
		}
		conditionalStack.push(conditionTrue, lineNumber)
		*shouldSkip = !conditionTrue
		return nil
	case "else":
		if strings.TrimSpace(arguments) != "" {
			return fmt.Errorf("invalid else command format: unexpected arguments")
		}
		if len(*conditionalStack) == 0 {
			return fmt.Errorf("else without a preceding if")
		}
		previousConditionalFrame, err := conditionalStack.peek()
		if err != nil {
			return err
		}
		if previousConditionalFrame.elseSeen {
			return fmt.Errorf("duplicate else for if block")
		}
		_, err = conditionalStack.pop()
		if err != nil {
			return err
		}
		previousIfState := previousConditionalFrame.active
		// If the previous 'if' was true, then the 'else' block should be skipped.
		// If the previous 'if' was false, the 'else' block should be executed,
		// but only if we are not already skipping due to an outer 'if'.
		if previousIfState { // Previous 'if' was true, so skip this 'else' block
			*shouldSkip = true
		} else { // Previous 'if' was false, so execute this 'else' block
			// Only set skip to false if no outer 'if' is currently skipping
			if len(*conditionalStack) > 0 {
				outerConditionalFrame, err := conditionalStack.peek()
				if err != nil {
					return err
				}
				*shouldSkip = !outerConditionalFrame.active // Revert to outer if's skip state
			} else {
				*shouldSkip = false // No outer if, so execute
			}
		}
		previousConditionalFrame.active = !previousIfState
		previousConditionalFrame.elseSeen = true
		*conditionalStack = append(*conditionalStack, previousConditionalFrame)
		return nil
	case "endif":
		if strings.TrimSpace(arguments) != "" {
			return fmt.Errorf("invalid endif command format: unexpected arguments")
		}
		if len(*conditionalStack) == 0 {
			return fmt.Errorf("endif without a preceding if")
		}
		_, err := conditionalStack.pop() // Pop from stack
		if err != nil {
			return err
		}
		if len(*conditionalStack) > 0 {
			currentConditionalFrame, err := conditionalStack.peek()
			if err != nil {
				return err
			}
			*shouldSkip = !currentConditionalFrame.active // Revert to parent if's skip state
		} else {
			*shouldSkip = false // No more if blocks, so no skipping
		}
		return nil
	}
	return nil
}

// handleOutputCommand substitutes and resolves arguments into outputFile, returning an error when arguments are empty.
// Inputs: arguments string representing output path, outputFile pointer to update, baseDir string, and parameters map.
// Outputs: returns nil on success or an error if arguments are empty.
func handleOutputCommand(arguments string, outputFile *string, baseDir string, parameters map[string]string) error {
	if strings.TrimSpace(arguments) == "" {
		return fmt.Errorf("invalid output command format: missing filename")
	}
	resolvedPath := substituteParams(arguments, parameters)
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(baseDir, resolvedPath)
	}
	*outputFile = resolvedPath
	return nil
}

// handleConcatCommand appends a parameter-substituted file item to itemsToConcat, returning an error when arguments are empty.
// Inputs: arguments string representing file path, itemsToConcat pointer to append to, baseDir string, and parameters map.
// Outputs: returns nil on success or an error if arguments are empty.
func handleConcatCommand(arguments string, itemsToConcat *[]ConcatItem, baseDir string, parameters map[string]string) error {
	if strings.TrimSpace(arguments) == "" {
		return fmt.Errorf("invalid concat command format: missing filename")
	}
	resolvedFilePath := substituteParams(arguments, parameters)
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: true, ContentOrPath: resolvedFilePath, BaseDir: baseDir})
	return nil
}

// handleIncludeCommand resolves and processes an included DSL file, updating shared output, items, parameters, and include state.
// Inputs: arguments string specifying included file, currentInstructionsFile string for relative resolution,
// outputFile pointer, itemsToConcat pointer, parameters map, activeIncludes map, and execution state pointer.
// Outputs: returns nil on success or a wrapped error with include context if resolution or processing fails.
func handleIncludeCommand(arguments string, currentInstructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, activeIncludes map[string]bool, state *executionState) error {
	if strings.TrimSpace(arguments) == "" {
		return fmt.Errorf("invalid include command format: missing filename")
	}
	includePath := substituteParams(arguments, parameters)
	targetFile := includePath
	if !filepath.IsAbs(targetFile) {
		targetFile = filepath.Join(filepath.Dir(currentInstructionsFile), targetFile)
	}
	err := processInstructions(targetFile, outputFile, itemsToConcat, parameters, filepath.Dir(targetFile), activeIncludes, state)
	if err != nil {
		return fmt.Errorf("error in include %s: %w", arguments, err)
	}
	return nil
}

// handleParamCommand defines a DSL parameter unless a CLI parameter or DSL set command has higher precedence.
// Inputs: arguments string in key=value format, parameters map to update, and execution state pointer.
// Outputs: returns nil on success or an error if the assignment syntax is invalid.
func handleParamCommand(arguments string, parameters map[string]string, state *executionState) error {
	parameterName, parameterValue, isValid := parseParameterAssignment(arguments)
	if isValid {
		// Perform substitution on the value before storing it
		substitutedValue := substituteParams(parameterValue, parameters)

		// Preserve only higher-precedence values; parameter-file values are defaults that param may replace.
		if !state.commandLineParameterNames[parameterName] && !state.dslSetParameterNames[parameterName] {
			parameters[parameterName] = substitutedValue
		}
	} else {
		return fmt.Errorf("invalid param command format: %s", arguments)
	}
	return nil
}

// handleSetCommand assigns a DSL parameter unless it originated from the CLI, returning invalid-assignment errors.
// Inputs: arguments string in key=value format, parameters map to update, and execution state pointer.
// Outputs: returns nil on success or an error if the assignment syntax is invalid.
func handleSetCommand(arguments string, parameters map[string]string, state *executionState) error {
	parameterName, parameterValue, isValid := parseParameterAssignment(arguments)
	if isValid {
		// Perform substitution on the value before storing it
		substitutedValue := substituteParams(parameterValue, parameters)

		// Only set the parameter if it was NOT set by a CLI --param flag
		if _, isCommandLineParameter := state.commandLineParameterNames[parameterName]; !isCommandLineParameter {
			parameters[parameterName] = substitutedValue
			state.dslSetParameterNames[parameterName] = true
		}
	} else {
		return fmt.Errorf("invalid set command format: %s", arguments)
	}
	return nil
}

// handlePrintCommand appends a parameter value as text or returns an error when the parameter is missing or invalid.
// Inputs: arguments string naming parameter, itemsToConcat pointer to append to, and parameters map.
// Outputs: returns nil on success or an error if arguments are empty or parameter not found.
func handlePrintCommand(arguments string, itemsToConcat *[]ConcatItem, parameters map[string]string) error {
	if strings.TrimSpace(arguments) == "" {
		return fmt.Errorf("invalid print command format: missing parameter name")
	}
	value, exists := parameters[arguments]
	if !exists {
		return fmt.Errorf("parameter not found: %s", arguments)
	}
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: false, ContentOrPath: value})
	return nil
}

// handleEmitCommand appends parameter-substituted literal text to itemsToConcat without returning a value.
// Inputs: arguments string to emit, itemsToConcat pointer to append to, and parameters map.
// Outputs/Side effects: appends ConcatItem to itemsToConcat.
func handleEmitCommand(arguments string, itemsToConcat *[]ConcatItem, parameters map[string]string) {
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: false, ContentOrPath: substituteParams(arguments, parameters)})
}

// dispatchCommand executes one normalized DSL line and returns its text-block mode or an execution error.
// Inputs: line string, instructionsFile path, lineNumber integer, outputFile pointer, itemsToConcat pointer,
// parameters map, baseDir string, currentPrefix pointer, conditionalStack pointer, shouldSkip pointer,
// activeIncludes map, and execution state pointer.
// Outputs: textBlockMode indicating whether a text block is started, and an error if command dispatch or execution fails.
func dispatchCommand(line string, instructionsFile string, lineNumber int, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string, currentPrefix *string, conditionalStack *conditionalStack, shouldSkip *bool, activeIncludes map[string]bool, state *executionState) (textBlockMode, error) {
	prefixIsActive := *currentPrefix != ""
	if prefixIsActive {
		prefixWithColon := *currentPrefix + ":"
		if strings.HasPrefix(line, prefixWithColon) {
			line = strings.TrimPrefix(line, prefixWithColon)
		} else {
			// If prefix is set, ignore all commands that don't have it.
			return textBlockNone, nil
		}
	}

	commandParts := strings.SplitN(line, " ", 2)
	command := commandParts[0]
	var arguments string
	if len(commandParts) > 1 {
		arguments = strings.TrimSpace(commandParts[1])
	}

	switch command {
	case "if", "else", "endif":
		return textBlockNone, handleConditionalCommand(command, arguments, parameters, conditionalStack, shouldSkip, lineNumber)
	}

	if *shouldSkip {
		if command == "text-begin" {
			return textBlockDiscard, nil
		}
		return textBlockNone, nil
	}

	if command == "set-prefix" {
		if arguments == "" {
			return textBlockNone, fmt.Errorf("invalid set-prefix command format: missing prefix")
		}
		*currentPrefix = arguments
		return textBlockNone, nil
	}

	if command == "clear-prefix" {
		if !prefixIsActive || arguments != "" {
			return textBlockNone, fmt.Errorf("invalid clear-prefix command format")
		}
		*currentPrefix = ""
		return textBlockNone, nil
	}

	switch command {
	case "output":
		return textBlockNone, handleOutputCommand(arguments, outputFile, baseDir, parameters)
	case "concat":
		return textBlockNone, handleConcatCommand(arguments, itemsToConcat, baseDir, parameters)
	case "include":
		return textBlockNone, handleIncludeCommand(arguments, instructionsFile, outputFile, itemsToConcat, parameters, activeIncludes, state)
	case "param":
		return textBlockNone, handleParamCommand(arguments, parameters, state)
	case "set":
		return textBlockNone, handleSetCommand(arguments, parameters, state)
	case "print":
		return textBlockNone, handlePrintCommand(arguments, itemsToConcat, parameters)
	case "emit":
		handleEmitCommand(arguments, itemsToConcat, parameters)
	case "text-begin":
		if arguments != "" {
			return textBlockNone, fmt.Errorf("invalid text-begin command format: unexpected arguments")
		}
		return textBlockOutput, nil
	default:
		return textBlockNone, fmt.Errorf("unknown command: %s", command)
	}
	return textBlockNone, nil
}

// processInstructions parses one DSL file into output and concatenation state, rejecting recursive includes and returning syntax or I/O errors.
// Inputs: instructionsFile path to parse, outputFile pointer, itemsToConcat pointer, parameters map,
// baseDir string for resolving relative paths, activeIncludes map, and execution state pointer.
// Outputs: returns nil on success or an error wrapped with file and line context if opening, reading, or processing fails.
func processInstructions(instructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string, activeIncludes map[string]bool, state *executionState) error {
	resolvedInstructionsFile, err := filepath.Abs(instructionsFile)
	if err != nil {
		return fmt.Errorf("error resolving instructions file %s: %w", instructionsFile, err)
	}
	// Resolve symlinks and junctions so aliases cannot bypass active-include cycle detection.
	resolvedInstructionsFile, err = filepath.EvalSymlinks(resolvedInstructionsFile)
	if err != nil {
		return fmt.Errorf("error resolving instructions file %s: %w", instructionsFile, err)
	}
	if activeIncludes[resolvedInstructionsFile] {
		return fmt.Errorf("include cycle detected: %s", resolvedInstructionsFile)
	}
	activeIncludes[resolvedInstructionsFile] = true
	defer delete(activeIncludes, resolvedInstructionsFile)

	file, err := os.Open(resolvedInstructionsFile)
	if err != nil {
		return fmt.Errorf("error opening instructions file %s: %w", instructionsFile, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Permit generated SQL text while retaining a bounded per-line memory limit.
	scanner.Buffer(make([]byte, 64*1024), maximumInputLineBytes)
	currentTextBlockMode := textBlockNone
	var textBlock strings.Builder

	conditionStack := conditionalStack{}
	shouldSkip := false
	var currentPrefix string
	lineNumber := 0
	textBlockStartLine := 0

	// Preserve literal text blocks while dispatching normalized command lines.
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		if currentTextBlockMode != textBlockNone {
			textEndCommand := "text-end"
			if currentPrefix != "" {
				textEndCommand = currentPrefix + ":text-end"
			}

			if strings.TrimSpace(line) == textEndCommand {
				if currentTextBlockMode == textBlockOutput {
					*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: false, ContentOrPath: substituteParams(textBlock.String(), parameters)})
				}
				currentTextBlockMode = textBlockNone
				textBlock.Reset()
			} else if currentTextBlockMode == textBlockOutput {
				textBlock.WriteString(line + "\n")
			}
			continue
		}

		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		newTextBlockMode, err := dispatchCommand(trimmedLine, instructionsFile, lineNumber, outputFile, itemsToConcat, parameters, baseDir, &currentPrefix, &conditionStack, &shouldSkip, activeIncludes, state)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", instructionsFile, lineNumber, err)
		}
		if newTextBlockMode != textBlockNone {
			textBlockStartLine = lineNumber
		}
		currentTextBlockMode = newTextBlockMode
	}

	// Check scanner errors immediately after scan loop finishes, before block checks.
	if scannerErr := scanner.Err(); scannerErr != nil {
		return fmt.Errorf("%s:%d: %w", instructionsFile, lineNumber+1, scannerErr)
	}

	if currentTextBlockMode != textBlockNone {
		return fmt.Errorf("%s:%d: unclosed text block", instructionsFile, textBlockStartLine)
	}

	if len(conditionStack) > 0 {
		lastFrame, _ := conditionStack.peek()
		return fmt.Errorf("%s:%d: unclosed if block(s)", instructionsFile, lastFrame.line)
	}

	return nil
}

// runConcat writes each planned file or text item to outputWriter and returns any read, copy, or write error.
// Inputs: outputWriter destination io.Writer, and itemsToConcat slice of concatenation items.
// Outputs: returns nil on success or an error if opening files, copying data, or writing text fails.
func runConcat(outputWriter io.Writer, itemsToConcat []ConcatItem) error {
	// Resolve and write each planned item in DSL order.
	for _, concatItem := range itemsToConcat {
		if concatItem.IsFile {
			// Keep concatenated filenames literal; @@ escapes apply only to generated text.
			resolvedPath := concatItem.ContentOrPath
			if !filepath.IsAbs(resolvedPath) {
				resolvedPath = filepath.Join(concatItem.BaseDir, resolvedPath)
			}

			sourceFile, err := os.Open(resolvedPath)
			if err != nil {
				return fmt.Errorf("error opening file %s: %w", resolvedPath, err)
			}

			// Close each source before advancing so large concatenations do not retain file handles.
			_, copyErr := io.Copy(outputWriter, sourceFile)
			closeErr := sourceFile.Close()
			if copyErr != nil {
				return fmt.Errorf("error copying from %s: %w", resolvedPath, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("error closing source file %s: %w", resolvedPath, closeErr)
			}
		} else {
			// Decode text escapes only when writing generated output.
			valueToWrite := unescapeString(concatItem.ContentOrPath)
			_, err := outputWriter.Write([]byte(valueToWrite))
			if err != nil {
				return fmt.Errorf("error writing text to output: %w", err)
			}
		}
	}

	return nil
}
