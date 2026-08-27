package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ConcatItem struct {
	IsFile  bool
	Value   string
	BaseDir string // Base directory used to resolve relative source-file paths.
}

var (
	paramFiles  string
	paramsSlice stringArray
	outputFlag  string
	cliParamsSet map[string]bool // Tracks parameters set by CLI --param.
	dslSetParams map[string]bool // Tracks parameters established by higher-precedence DSL set commands.
)

// init registers command-line options and initializes global precedence state; it has no return value.
func init() {
	flag.StringVar(&paramFiles, "param-file", "", "Comma-separated list of parameter files (key=value per line)")
	flag.Var(&paramsSlice, "param", "Key-value pair parameter (e.g., --param key=value). Can be specified multiple times.")
	flag.StringVar(&outputFlag, "output", "", "Output file path. If not specified, output goes to stdout.")
	cliParamsSet = make(map[string]bool) // Initialize the map
	dslSetParams = make(map[string]bool)
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
	if paramFiles != "" {
		files := strings.Split(paramFiles, ",")
		for _, file := range files {
			err := loadParamsFromFile(file, parameters)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading parameters from file %s: %v\n", file, err)
				os.Exit(1)
			}
		}
	}

	// Load parameters from command line (highest precedence) before processing DSL instructions
	// Apply CLI values before DSL parsing so DSL commands cannot override them.
	for _, p := range paramsSlice {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 2 {
			parameters[parts[0]] = parts[1]
			cliParamsSet[parts[0]] = true // Mark this parameter as set by CLI
		}
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
	if outputFlag != "" {
		finalOutputFile = outputFlag
	}

	var outputWriter io.Writer
	if finalOutputFile == "" {
		outputWriter = os.Stdout
	} else {
		outFile, err := os.Create(finalOutputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file %s: %v\n", finalOutputFile, err)
			os.Exit(1)
		}
		defer outFile.Close()
		outputWriter = outFile
	}

	// Materialize the complete plan only after all instructions have parsed successfully.
	err = runConcat(outputWriter, itemsToConcat, parameters)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during concatenation: %v\n", err)
		os.Exit(1)
	}

}

// loadParamsFromFile reads key=value entries from filename into parameters and returns parsing or I/O errors.
func loadParamsFromFile(filename string, parameters map[string]string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening parameter file %s: %v", filename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Skip comments and blank lines while preserving literal parameter values.
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			parameters[parts[0]] = parts[1]
		} else {
			return fmt.Errorf("invalid parameter file line format: %s", line)
		}
	}
	return scanner.Err()
}

type stringArray []string

// String returns the flag values as a comma-separated string for flag.Value display.
func (i *stringArray) String() string {
	return strings.Join(*i, ",")
}

// Set appends one command-line flag value and always returns nil.
func (i *stringArray) Set(value string) error {
	*i = append(*i, value)
	return nil
}

// substituteParams replaces known ${key} references in s using parameters and returns the substituted text.
func substituteParams(s string, parameters map[string]string) string {
	result := s
	// Substitute every currently defined key without recursively expanding replacement values.
	for key, value := range parameters {
		result = strings.ReplaceAll(result, "$"+"{"+key+"}", value)
	}
	return result
}

// unescapeString converts DSL @@ escape sequences in s and returns the decoded text.
func unescapeString(s string) string {
	s = strings.ReplaceAll(s, "@@n", "\n")
	s = strings.ReplaceAll(s, "@@r", "\r")
	s = strings.ReplaceAll(s, "@@t", "\t")
	s = strings.ReplaceAll(s, "@@s", " ")
	return s
}

type ifFrame struct {
	active   bool
	elseSeen bool
}

type ifStack []ifFrame

// push adds one conditional execution state to s without returning a value.
func (s *ifStack) push(val bool) {
	*s = append(*s, ifFrame{active: val})
}

// pop removes and returns the most recent conditional state, or returns an error when s is empty.
func (s *ifStack) pop() (ifFrame, error) {
	if len(*s) == 0 {
		return ifFrame{}, fmt.Errorf("pop on empty stack")
	}
	val := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return val, nil
}

// peek returns the most recent conditional state without removing it, or an error when s is empty.
func (s *ifStack) peek() (ifFrame, error) {
	if len(*s) == 0 {
		return ifFrame{}, fmt.Errorf("peek on empty stack")
	}
	return (*s)[len(*s)-1], nil
}

// evaluateCondition evaluates a DSL comparison against parameters and returns its result or a format error.
func evaluateCondition(condition string, parameters map[string]string) (bool, error) {
	operators := []string{">=", "<=", "=", ">", "<"}
	var operator, key, expectedValue string

	// Prefer multi-character operators so their leading character is not parsed first.
	for _, op := range operators {
		if strings.Contains(condition, op) {
			parts := strings.SplitN(condition, op, 2)
			if len(parts) == 2 {
				operator = op
				key = parts[0]
				expectedValue = parts[1]
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
	actualNum, err1 := strconv.ParseFloat(actualValue, 64)
	expectedNum, err2 := strconv.ParseFloat(expectedValue, 64)

	if err1 != nil || err2 != nil {
		return false, nil // One of the values is not a number, so comparison is false
	}

	switch operator {
	case ">":
		return actualNum > expectedNum, nil
	case ">=":
		return actualNum >= expectedNum, nil
	case "<":
		return actualNum < expectedNum, nil
	case "<=":
		return actualNum <= expectedNum, nil
	}

	return false, fmt.Errorf("unhandled operator: %s", operator)
}

// handleConditionalCommand updates ifStk and skip for an if, else, or endif command and returns validation errors.
func handleConditionalCommand(command, args string, parameters map[string]string, ifStk *ifStack, skip *bool) error {
	switch command {
	case "if":
		if strings.TrimSpace(args) == "" {
			return fmt.Errorf("invalid if command format: missing condition")
		}
		if *skip { // If already skipping, push false to stack and continue skipping
			ifStk.push(false)
			return nil
		}
		conditionTrue, err := evaluateCondition(args, parameters)
		if err != nil {
			return err
		}
		ifStk.push(conditionTrue)
		*skip = !conditionTrue
		return nil
	case "else":
		if strings.TrimSpace(args) != "" {
			return fmt.Errorf("invalid else command format: unexpected arguments")
		}
		if len(*ifStk) == 0 {
			return fmt.Errorf("else without a preceding if")
		}
		prevIfFrame, err := ifStk.peek()
		if err != nil {
			return err
		}
		if prevIfFrame.elseSeen {
			return fmt.Errorf("duplicate else for if block")
		}
		_, err = ifStk.pop()
		if err != nil {
			return err
		}
		prevIfState := prevIfFrame.active
		// If the previous 'if' was true, then the 'else' block should be skipped.
		// If the previous 'if' was false, the 'else' block should be executed,
		// but only if we are not already skipping due to an outer 'if'.
		if prevIfState { // Previous 'if' was true, so skip this 'else' block
			*skip = true
		} else { // Previous 'if' was false, so execute this 'else' block
			// Only set skip to false if no outer 'if' is currently skipping
			if len(*ifStk) > 0 {
				outerIfFrame, err := ifStk.peek()
				if err != nil {
					return err
				}
				*skip = !outerIfFrame.active // Revert to outer if's skip state
			} else {
				*skip = false // No outer if, so execute
			}
		}
		prevIfFrame.active = !prevIfState
		prevIfFrame.elseSeen = true
		*ifStk = append(*ifStk, prevIfFrame)
		return nil
	case "endif":
		if strings.TrimSpace(args) != "" {
			return fmt.Errorf("invalid endif command format: unexpected arguments")
		}
		if len(*ifStk) == 0 {
			return fmt.Errorf("endif without a preceding if")
		}
		_, err := ifStk.pop() // Pop from stack
		if err != nil {
			return err
		}
		if len(*ifStk) > 0 {
			currentIfFrame, err := ifStk.peek()
			if err != nil {
				return err
			}
			*skip = !currentIfFrame.active // Revert to parent if's skip state
		} else {
			*skip = false // No more if blocks, so no skipping
		}
		return nil
	}
	return nil
}

// handleOutputCommand substitutes and resolves args into outputFile; callers must validate nonempty args.
func handleOutputCommand(args string, outputFile *string, baseDir string, parameters map[string]string) {
	if strings.TrimSpace(args) == "" {
		panic("handleOutputCommand called with empty args")
	}
	resolvedPath := substituteParams(args, parameters)
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(baseDir, resolvedPath)
	}
	*outputFile = resolvedPath
}

// handleConcatCommand appends a parameter-substituted file item to itemsToConcat; callers must validate nonempty args.
func handleConcatCommand(args string, itemsToConcat *[]ConcatItem, baseDir string, parameters map[string]string) {
	if strings.TrimSpace(args) == "" {
		panic("handleConcatCommand called with empty args")
	}
	resolvedValue := substituteParams(args, parameters)
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: true, Value: resolvedValue, BaseDir: baseDir})
}

// handleIncludeCommand resolves and processes an included DSL file, updating shared output, items, parameters, and include state.
func handleIncludeCommand(args string, currentInstructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string, activeIncludes map[string]bool) error {
	if strings.TrimSpace(args) == "" {
		return fmt.Errorf("invalid include command format: missing filename")
	}
	includePath := substituteParams(args, parameters)
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
func handleParamCommand(args string, parameters map[string]string) error {
	paramParts := strings.SplitN(args, "=", 2)
	if len(paramParts) == 2 {
		paramName := paramParts[0]
		paramValue := paramParts[1] // This is the value that needs substitution

		// Perform substitution on the value before storing it
		substitutedValue := substituteParams(paramValue, parameters)

		// Preserve only higher-precedence values; parameter-file values are defaults that param may replace.
		if !cliParamsSet[paramName] && !dslSetParams[paramName] {
			parameters[paramName] = substitutedValue
		}
	} else {
		return fmt.Errorf("invalid param command format: %s", args)
	}
	return nil
}

// handleSetCommand assigns a DSL parameter unless it originated from the CLI, returning invalid-assignment errors.
func handleSetCommand(args string, parameters map[string]string) error {
	setParts := strings.SplitN(args, "=", 2)
	if len(setParts) == 2 {
		paramName := setParts[0]
		paramValue := setParts[1] // This is the value that needs substitution

		// Perform substitution on the value before storing it
		substitutedValue := substituteParams(paramValue, parameters)

		// Only set the parameter if it was NOT set by a CLI --param flag
		if _, isCliParam := cliParamsSet[paramName]; !isCliParam {
			parameters[paramName] = substitutedValue
			dslSetParams[paramName] = true
		}
	} else {
		return fmt.Errorf("invalid set command format: %s", args)
	}
	return nil
}

// handlePrintCommand appends a parameter value as text or returns an error when the parameter is missing or invalid.
func handlePrintCommand(args string, itemsToConcat *[]ConcatItem, parameters map[string]string) error {
	if strings.TrimSpace(args) == "" {
		return fmt.Errorf("invalid print command format: missing parameter name")
	}
	value, exists := parameters[args]
	if !exists {
		return fmt.Errorf("parameter not found: %s", args)
	}
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: false, Value: value})
	return nil
}

// handleEmitCommand appends parameter-substituted literal text to itemsToConcat without returning a value.
func handleEmitCommand(args string, itemsToConcat *[]ConcatItem, parameters map[string]string) {
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: false, Value: substituteParams(args, parameters)})
}

// dispatchCommand executes one normalized DSL line and reports whether it begins a text block or returns an error.
func dispatchCommand(line string, instructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string, currentPrefix *string, ifStk *ifStack, skip *bool, activeIncludes map[string]bool) (bool, error) {
	textBegan := false // Tracks whether this command begins a text block.
	if *currentPrefix != "" {
		prefixWithColon := *currentPrefix + ":"
		if strings.HasPrefix(line, prefixWithColon) {
			if line == prefixWithColon+"clear-prefix" {
				*currentPrefix = ""
				return textBegan, nil
			}
			line = strings.TrimPrefix(line, prefixWithColon)
		} else {
			// If prefix is set, ignore all commands that don't have it
			return textBegan, nil
		}
	}

	parts := strings.SplitN(line, " ", 2)
	command := parts[0]
	var args string
	if len(parts) > 1 {
		args = parts[1]
	}

	switch command {
	case "if", "else", "endif":
		return textBegan, handleConditionalCommand(command, args, parameters, ifStk, skip)
	}

	if command == "set-prefix" {
		if strings.TrimSpace(args) == "" {
			return textBegan, fmt.Errorf("invalid set-prefix command format: missing prefix")
		}
		*currentPrefix = args
		return textBegan, nil
	}

	if *skip {
		return textBegan, nil
	}

	switch command {
	case "output":
		if strings.TrimSpace(args) == "" {
			return textBegan, fmt.Errorf("invalid output command format: missing filename")
		}
		handleOutputCommand(args, outputFile, baseDir, parameters)
	case "concat":
		if strings.TrimSpace(args) == "" {
			return textBegan, fmt.Errorf("invalid concat command format: missing filename")
		}
		handleConcatCommand(args, itemsToConcat, baseDir, parameters)
	case "include":
		return textBegan, handleIncludeCommand(args, instructionsFile, outputFile, itemsToConcat, parameters, baseDir, activeIncludes)
	case "param":
		return textBegan, handleParamCommand(args, parameters)
	case "set":
		return textBegan, handleSetCommand(args, parameters)
	case "print":
		return textBegan, handlePrintCommand(args, itemsToConcat, parameters)
	case "emit":
		handleEmitCommand(args, itemsToConcat, parameters)
	case "text-begin":
		if strings.TrimSpace(args) != "" {
			return textBegan, fmt.Errorf("invalid text-begin command format: unexpected arguments")
		}
		textBegan = true
	default:
		return textBegan, fmt.Errorf("unknown command: %s", command)
	}
	return textBegan, nil
}

// processInstructions parses one DSL file into output and concatenation state, rejecting recursive includes and returning syntax or I/O errors.
func processInstructions(instructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string, activeIncludes map[string]bool) error {
	resolvedInstructionsFile, err := filepath.Abs(instructionsFile)
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
	inTextBlock := false
	var textBlock strings.Builder

	ifStk := ifStack{}
	skip := false
	var currentPrefix string

	// Preserve literal text blocks while dispatching normalized command lines.
	for scanner.Scan() {
		line := scanner.Text()

		if inTextBlock {
			textEndCommand := "text-end"
			if currentPrefix != "" {
				textEndCommand = currentPrefix + ":text-end"
			}

			if strings.TrimSpace(line) == textEndCommand {
				*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: false, Value: substituteParams(textBlock.String(), parameters)})
				inTextBlock = false
				textBlock.Reset()
			} else {
				textBlock.WriteString(line + "\n")
			}
			continue
		}

		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}

		textBegan, err := dispatchCommand(trimmedLine, resolvedInstructionsFile, outputFile, itemsToConcat, parameters, baseDir, &currentPrefix, &ifStk, &skip, activeIncludes)
		if err != nil {
			return err
		}
		inTextBlock = textBegan
	}

	if inTextBlock {
		return fmt.Errorf("unclosed text block")
	}

	if len(ifStk) > 0 {
		return fmt.Errorf("unclosed if block(s)")
	}

	return scanner.Err()
}

// runConcat writes each planned file or text item to outputWriter and returns any read, copy, or write error.
func runConcat(outputWriter io.Writer, itemsToConcat []ConcatItem, parameters map[string]string) error {
	// Resolve and write each planned item in DSL order.
	for _, item := range itemsToConcat {
		if item.IsFile {
			// Keep concatenated filenames literal; @@ escapes apply only to generated text.
			resolvedPath := item.Value
			if !filepath.IsAbs(resolvedPath) {
				resolvedPath = filepath.Join(item.BaseDir, resolvedPath)
			}

			sourceFile, err := os.Open(resolvedPath)
			if err != nil {
				return fmt.Errorf("error opening file %s: %v", resolvedPath, err)
			}
			defer sourceFile.Close()

			_, err = io.Copy(outputWriter, sourceFile)
			if err != nil {
				return fmt.Errorf("error copying from %s: %v", resolvedPath, err)
			}
		} else {
			// Decode text escapes only when writing generated output.
			valueToWrite := unescapeString(item.Value)
			_, err := outputWriter.Write([]byte(valueToWrite))
			if err != nil {
				return fmt.Errorf("error writing text to output: %v", err)
			}
		}
	}

	// No success message for stdout to avoid polluting output
	if outputWriter != os.Stdout {
		fmt.Fprintf(os.Stdout, "Successfully concatenated files to output.\n")
	}
	return nil
}
