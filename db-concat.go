package main

import (
	"bufio"
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

var (
	parameterFileList          string
	commandLineParameterValues stringList
	commandLineOutputFile      string
	commandLineParameterNames  map[string]bool // Tracks parameters set by CLI --param.
	dslSetParameterNames       map[string]bool // Tracks parameters established by higher-precedence DSL set commands.
)

// init registers command-line options and initializes global precedence state; it has no return value.
func init() {
	flag.StringVar(&parameterFileList, "param-file", "", "Comma-separated list of parameter files (key=value per line)")
	flag.Var(&commandLineParameterValues, "param", "Key-value pair parameter (e.g., --param key=value). Can be specified multiple times.")
	flag.StringVar(&commandLineOutputFile, "output", "", "Output file path. If not specified, output goes to stdout.")
	commandLineParameterNames = make(map[string]bool)
	dslSetParameterNames = make(map[string]bool)
}

// main parses inputs, builds the concatenation plan, writes the requested output, and exits nonzero on failure.
func main() {
	// Parse flags before establishing the precedence-aware parameter state.
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Usage: db-concat [OPTIONS] <instructions_file>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	instructionsFile := flag.Arg(0)
	instructionsDir := filepath.Dir(instructionsFile)
	if instructionsDir == "" {
		instructionsDir = "."
	}
	parameters := make(map[string]string)

	// Load parameters from files (lowest precedence)
	if parameterFileList != "" {
		parameterFiles := strings.Split(parameterFileList, ",")
		for _, parameterFile := range parameterFiles {
			err := loadParamsFromFile(parameterFile, parameters)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading parameters from file %s: %v\n", parameterFile, err)
				os.Exit(1)
			}
		}
	}

	// Load parameters from command line (highest precedence) before processing DSL instructions
	// Apply CLI values before DSL parsing so DSL commands cannot override them.
	for _, parameterArgument := range commandLineParameterValues {
		parameterName, parameterValue, isValid := parseParameterAssignment(parameterArgument)
		if !isValid {
			fmt.Fprintf(os.Stderr, "Invalid --param value %q: expected key=value with a non-empty key\n", parameterArgument)
			os.Exit(1)
		}
		parameters[parameterName] = parameterValue
		commandLineParameterNames[parameterName] = true
	}

	var dslOutputFile string
	var itemsToConcat []ConcatItem

	activeIncludes := make(map[string]bool)
	err := processInstructions(instructionsFile, &dslOutputFile, &itemsToConcat, parameters, instructionsDir, activeIncludes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing instructions: %v\n", err)
		os.Exit(1)
	}

	finalOutputFile := dslOutputFile
	// An explicit CLI destination overrides the instruction-file default.
	if commandLineOutputFile != "" {
		finalOutputFile = commandLineOutputFile
	}

	// Materialize the complete plan without replacing a file destination until every item succeeds.
	err = writeOutput(finalOutputFile, itemsToConcat, parameters)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

}

// writeOutput writes planned items to stdout or atomically replaces finalOutputFile after a successful file write.
func writeOutput(finalOutputFile string, itemsToConcat []ConcatItem, parameters map[string]string) error {
	if finalOutputFile == "" {
		return runConcat(os.Stdout, itemsToConcat, parameters)
	}

	// Preserve an existing destination's permissions, or use a predictable readable default for a new file.
	outputPermission := defaultOutputPermission
	existingOutputInfo, statErr := os.Stat(finalOutputFile)
	if statErr == nil {
		outputPermission = existingOutputInfo.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("error inspecting output file %s: %v", finalOutputFile, statErr)
	}

	// Create the temporary file beside the destination so a successful rename stays on the same filesystem.
	temporaryOutput, err := os.CreateTemp(filepath.Dir(finalOutputFile), "."+filepath.Base(finalOutputFile)+".tmp-*")
	if err != nil {
		return fmt.Errorf("error creating temporary output file for %s: %v", finalOutputFile, err)
	}
	temporaryOutputPath := temporaryOutput.Name()
	defer func() {
		temporaryOutput.Close()
		os.Remove(temporaryOutputPath)
	}()
	if err := temporaryOutput.Chmod(outputPermission); err != nil {
		return fmt.Errorf("error setting permissions on temporary output file %s: %v", temporaryOutputPath, err)
	}

	if err := runConcat(temporaryOutput, itemsToConcat, parameters); err != nil {
		return err
	}
	if err := temporaryOutput.Close(); err != nil {
		return fmt.Errorf("error closing temporary output file %s: %v", temporaryOutputPath, err)
	}
	if err := os.Rename(temporaryOutputPath, finalOutputFile); err != nil {
		return fmt.Errorf("error replacing output file %s: %v", finalOutputFile, err)
	}

	fmt.Fprintln(os.Stdout, "Successfully concatenated files to output.")
	return nil
}

// loadParamsFromFile reads key=value entries from filename into parameters and returns parsing or I/O errors.
func loadParamsFromFile(filename string, parameters map[string]string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening parameter file %s: %v", filename, err)
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
	return scanner.Err()
}

// parseParameterAssignment parses key=value text, trims the key, and reports whether the key is nonempty.
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
func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

// Set appends one command-line flag value and always returns nil.
func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

// substituteParams replaces known ${key} references in text using parameters and returns the substituted text.
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
func unescapeString(text string) string {
	text = strings.ReplaceAll(text, "@@n", "\n")
	text = strings.ReplaceAll(text, "@@r", "\r")
	text = strings.ReplaceAll(text, "@@t", "\t")
	text = strings.ReplaceAll(text, "@@s", " ")
	return text
}

type conditionalFrame struct {
	active   bool
	elseSeen bool
}

type conditionalStack []conditionalFrame

// push adds one conditional execution state to stack without returning a value.
func (stack *conditionalStack) push(isActive bool) {
	*stack = append(*stack, conditionalFrame{active: isActive})
}

// pop removes and returns the most recent conditional state, or returns an error when stack is empty.
func (stack *conditionalStack) pop() (conditionalFrame, error) {
	if len(*stack) == 0 {
		return conditionalFrame{}, fmt.Errorf("pop on empty stack")
	}
	activeFrame := (*stack)[len(*stack)-1]
	*stack = (*stack)[:len(*stack)-1]
	return activeFrame, nil
}

// peek returns the most recent conditional state without removing it, or an error when stack is empty.
func (stack *conditionalStack) peek() (conditionalFrame, error) {
	if len(*stack) == 0 {
		return conditionalFrame{}, fmt.Errorf("peek on empty stack")
	}
	return (*stack)[len(*stack)-1], nil
}

// evaluateCondition evaluates a DSL comparison against parameters and returns its result or a format error.
func evaluateCondition(condition string, parameters map[string]string) (bool, error) {
	operators := []string{">=", "<=", "=", ">", "<"}
	var operator, key, expectedValue string

	// Prefer multi-character operators so their leading character is not parsed first.
	for _, op := range operators {
		if strings.Contains(condition, op) {
			conditionParts := strings.SplitN(condition, op, 2)
			if len(conditionParts) == 2 {
				operator = op
				key = conditionParts[0]
				expectedValue = conditionParts[1]
				break
			}
		}
	}

	if operator == "" {
		return false, fmt.Errorf("invalid condition format: %s", condition)
	}

	actualValue, ok := parameters[key]
	if !ok {
		return false, nil // Key not found, condition is false
	}

	if operator == "=" {
		return actualValue == expectedValue, nil
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
func handleConditionalCommand(command, arguments string, parameters map[string]string, conditionalStack *conditionalStack, shouldSkip *bool) error {
	switch command {
	case "if":
		if strings.TrimSpace(arguments) == "" {
			return fmt.Errorf("invalid if command format: missing condition")
		}
		if *shouldSkip { // If already skipping, push false to stack and continue skipping
			conditionalStack.push(false)
			return nil
		}
		conditionTrue, err := evaluateCondition(arguments, parameters)
		if err != nil {
			return err
		}
		conditionalStack.push(conditionTrue)
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

// handleOutputCommand substitutes and resolves arguments into outputFile; callers must validate nonempty arguments.
func handleOutputCommand(arguments string, outputFile *string, baseDir string, parameters map[string]string) {
	if strings.TrimSpace(arguments) == "" {
		panic("handleOutputCommand called with empty args")
	}
	resolvedPath := substituteParams(arguments, parameters)
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(baseDir, resolvedPath)
	}
	*outputFile = resolvedPath
}

// handleConcatCommand appends a parameter-substituted file item to itemsToConcat; callers must validate nonempty arguments.
func handleConcatCommand(arguments string, itemsToConcat *[]ConcatItem, baseDir string, parameters map[string]string) {
	if strings.TrimSpace(arguments) == "" {
		panic("handleConcatCommand called with empty args")
	}
	resolvedFilePath := substituteParams(arguments, parameters)
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: true, ContentOrPath: resolvedFilePath, BaseDir: baseDir})
}

// handleIncludeCommand resolves and processes an included DSL file, updating shared output, items, parameters, and include state.
func handleIncludeCommand(arguments string, currentInstructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string, activeIncludes map[string]bool) error {
	if strings.TrimSpace(arguments) == "" {
		return fmt.Errorf("invalid include command format: missing filename")
	}
	includePath := substituteParams(arguments, parameters)
	if !filepath.IsAbs(includePath) {
		absPath, err := filepath.Abs(filepath.Join(filepath.Dir(currentInstructionsFile), includePath))
		if err != nil {
			return fmt.Errorf("error resolving absolute path for %s: %v", includePath, err)
		}
		includePath = absPath
	}
	err := processInstructions(includePath, outputFile, itemsToConcat, parameters, filepath.Dir(includePath), activeIncludes)
	if err != nil {
		return err
	}
	return nil
}

// handleParamCommand defines a DSL parameter unless a CLI parameter or DSL set command has higher precedence.
func handleParamCommand(arguments string, parameters map[string]string) error {
	parameterName, parameterValue, isValid := parseParameterAssignment(arguments)
	if isValid {

		// Perform substitution on the value before storing it
		substitutedValue := substituteParams(parameterValue, parameters)

		// Preserve only higher-precedence values; parameter-file values are defaults that param may replace.
		if !commandLineParameterNames[parameterName] && !dslSetParameterNames[parameterName] {
			parameters[parameterName] = substitutedValue
		}
	} else {
		return fmt.Errorf("invalid param command format: %s", arguments)
	}
	return nil
}

// handleSetCommand assigns a DSL parameter unless it originated from the CLI, returning invalid-assignment errors.
func handleSetCommand(arguments string, parameters map[string]string) error {
	parameterName, parameterValue, isValid := parseParameterAssignment(arguments)
	if isValid {

		// Perform substitution on the value before storing it
		substitutedValue := substituteParams(parameterValue, parameters)

		// Only set the parameter if it was NOT set by a CLI --param flag
		if _, isCommandLineParameter := commandLineParameterNames[parameterName]; !isCommandLineParameter {
			parameters[parameterName] = substitutedValue
			dslSetParameterNames[parameterName] = true
		}
	} else {
		return fmt.Errorf("invalid set command format: %s", arguments)
	}
	return nil
}

// handlePrintCommand appends a parameter value as text or returns an error when the parameter is missing or invalid.
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
func handleEmitCommand(arguments string, itemsToConcat *[]ConcatItem, parameters map[string]string) {
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: false, ContentOrPath: substituteParams(arguments, parameters)})
}

// dispatchCommand executes one normalized DSL line and returns its text-block mode or an error.
func dispatchCommand(line string, instructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string, currentPrefix *string, conditionalStack *conditionalStack, shouldSkip *bool, activeIncludes map[string]bool) (textBlockMode, error) {
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
		arguments = commandParts[1]
	}

	switch command {
	case "if", "else", "endif":
		return textBlockNone, handleConditionalCommand(command, arguments, parameters, conditionalStack, shouldSkip)
	}

	if *shouldSkip {
		if command == "text-begin" {
			return textBlockDiscard, nil
		}
		return textBlockNone, nil
	}

	if command == "set-prefix" {
		if strings.TrimSpace(arguments) == "" {
			return textBlockNone, fmt.Errorf("invalid set-prefix command format: missing prefix")
		}
		*currentPrefix = arguments
		return textBlockNone, nil
	}

	if command == "clear-prefix" {
		if !prefixIsActive || strings.TrimSpace(arguments) != "" {
			return textBlockNone, fmt.Errorf("invalid clear-prefix command format")
		}
		*currentPrefix = ""
		return textBlockNone, nil
	}

	switch command {
	case "output":
		if strings.TrimSpace(arguments) == "" {
			return textBlockNone, fmt.Errorf("invalid output command format: missing filename")
		}
		handleOutputCommand(arguments, outputFile, baseDir, parameters)
	case "concat":
		if strings.TrimSpace(arguments) == "" {
			return textBlockNone, fmt.Errorf("invalid concat command format: missing filename")
		}
		handleConcatCommand(arguments, itemsToConcat, baseDir, parameters)
	case "include":
		return textBlockNone, handleIncludeCommand(arguments, instructionsFile, outputFile, itemsToConcat, parameters, baseDir, activeIncludes)
	case "param":
		return textBlockNone, handleParamCommand(arguments, parameters)
	case "set":
		return textBlockNone, handleSetCommand(arguments, parameters)
	case "print":
		return textBlockNone, handlePrintCommand(arguments, itemsToConcat, parameters)
	case "emit":
		handleEmitCommand(arguments, itemsToConcat, parameters)
	case "text-begin":
		if strings.TrimSpace(arguments) != "" {
			return textBlockNone, fmt.Errorf("invalid text-begin command format: unexpected arguments")
		}
		return textBlockOutput, nil
	default:
		return textBlockNone, fmt.Errorf("unknown command: %s", command)
	}
	return textBlockNone, nil
}

// processInstructions parses one DSL file into output and concatenation state, rejecting recursive includes and returning syntax or I/O errors.
func processInstructions(instructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string, activeIncludes map[string]bool) error {
	resolvedInstructionsFile, err := filepath.Abs(instructionsFile)
	if err != nil {
		return fmt.Errorf("error resolving instructions file %s: %v", instructionsFile, err)
	}
	// Resolve symlinks and junctions so aliases cannot bypass active-include cycle detection.
	resolvedInstructionsFile, err = filepath.EvalSymlinks(resolvedInstructionsFile)
	if err != nil {
		return fmt.Errorf("error resolving instructions file %s: %v", instructionsFile, err)
	}
	if activeIncludes[resolvedInstructionsFile] {
		return fmt.Errorf("include cycle detected: %s", resolvedInstructionsFile)
	}
	activeIncludes[resolvedInstructionsFile] = true
	defer delete(activeIncludes, resolvedInstructionsFile)

	file, err := os.Open(resolvedInstructionsFile)
	if err != nil {
		return fmt.Errorf("error opening instructions file %s: %v", instructionsFile, err)
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

	// Preserve literal text blocks while dispatching normalized command lines.
	for scanner.Scan() {
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

		newTextBlockMode, err := dispatchCommand(trimmedLine, resolvedInstructionsFile, outputFile, itemsToConcat, parameters, baseDir, &currentPrefix, &conditionStack, &shouldSkip, activeIncludes)
		if err != nil {
			return err
		}
		currentTextBlockMode = newTextBlockMode
	}

	if currentTextBlockMode != textBlockNone {
		return fmt.Errorf("unclosed text block")
	}

	if len(conditionStack) > 0 {
		return fmt.Errorf("unclosed if block(s)")
	}

	return scanner.Err()
}

// runConcat writes each planned file or text item to outputWriter and returns any read, copy, or write error.
func runConcat(outputWriter io.Writer, itemsToConcat []ConcatItem, parameters map[string]string) error {
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
				return fmt.Errorf("error opening file %s: %v", resolvedPath, err)
			}

			// Close each source before advancing so large concatenations do not retain file handles.
			_, copyErr := io.Copy(outputWriter, sourceFile)
			closeErr := sourceFile.Close()
			if copyErr != nil {
				return fmt.Errorf("error copying from %s: %v", resolvedPath, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("error closing source file %s: %v", resolvedPath, closeErr)
			}
		} else {
			// Decode text escapes only when writing generated output.
			valueToWrite := unescapeString(concatItem.ContentOrPath)
			_, err := outputWriter.Write([]byte(valueToWrite))
			if err != nil {
				return fmt.Errorf("error writing text to output: %v", err)
			}
		}
	}

	return nil
}
