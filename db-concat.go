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
	BaseDir string // New field to store the base directory for path resolution
}

var (
	paramFiles  string
	paramsSlice stringArray
	outputFlag  string
	cliParamsSet map[string]bool // New: To track parameters set by CLI --param
)

func init() {
	flag.StringVar(&paramFiles, "param-file", "", "Comma-separated list of parameter files (key=value per line)")
	flag.Var(&paramsSlice, "param", "Key-value pair parameter (e.g., --param key=value). Can be specified multiple times.")
	flag.StringVar(&outputFlag, "output", "", "Output file path. If not specified, output goes to stdout.")
	cliParamsSet = make(map[string]bool) // Initialize the map
}

func main() {
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
	for _, p := range paramsSlice {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 2 {
			parameters[parts[0]] = parts[1]
			cliParamsSet[parts[0]] = true // Mark this parameter as set by CLI
		}
	}

	var dslOutputFile string
	var itemsToConcat []ConcatItem

	err := processInstructions(instructionsFile, &dslOutputFile, &itemsToConcat, parameters, instructionsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing instructions: %v\n", err)
		os.Exit(1)
	}

	// Re-substitute now that all parameters are finalized
	for i := range itemsToConcat {
		itemsToConcat[i].Value = substituteParams(itemsToConcat[i].Value, parameters)
	}
	if dslOutputFile != "" {
		dslOutputFile = substituteParams(dslOutputFile, parameters)
	}

	finalOutputFile := outputFlag
	if dslOutputFile != "" {
		finalOutputFile = dslOutputFile // DSL 'output' command overrides command-line flag
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

	err = runConcat(outputWriter, itemsToConcat, parameters)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during concatenation: %v\n", err)
		os.Exit(1)
	}

}

func loadParamsFromFile(filename string, parameters map[string]string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening parameter file %s: %v", filename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
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

func (i *stringArray) String() string {
	return strings.Join(*i, ",")
}

func (i *stringArray) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func substituteParams(s string, parameters map[string]string) string {
	result := s
	for key, value := range parameters {
		result = strings.ReplaceAll(result, "$"+"{"+key+"}", value)
	}
	return result
}

func unescapeString(s string) string {
	s = strings.ReplaceAll(s, "@@n", "\n")
	s = strings.ReplaceAll(s, "@@r", "\r")
	s = strings.ReplaceAll(s, "@@t", "\t")
	s = strings.ReplaceAll(s, "@@s", " ")
	return s
}

type ifStack []bool

func (s *ifStack) push(val bool) {
	*s = append(*s, val)
}

func (s *ifStack) pop() (bool, error) {
	if len(*s) == 0 {
		return false, fmt.Errorf("pop on empty stack")
	}
	val := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return val, nil
}

func (s *ifStack) peek() (bool, error) {
	if len(*s) == 0 {
		return false, fmt.Errorf("peek on empty stack")
	}
	return (*s)[len(*s)-1], nil
}

func evaluateCondition(condition string, parameters map[string]string) (bool, error) {
	operators := []string{">=", "<=", "=", ">", "<"}
	var operator, key, expectedValue string

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

func handleConditionalCommand(command, args string, parameters map[string]string, ifStk *ifStack, skip *bool) error {
	switch command {
	case "if":
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
		if len(*ifStk) == 0 {
			return fmt.Errorf("else without a preceding if")
		}
		prevIfState, err := ifStk.pop()
		if err != nil {
			return err
		}
		// If the previous 'if' was true, then the 'else' block should be skipped.
		// If the previous 'if' was false, the 'else' block should be executed,
		// but only if we are not already skipping due to an outer 'if'.
		if prevIfState { // Previous 'if' was true, so skip this 'else' block
			*skip = true
		} else { // Previous 'if' was false, so execute this 'else' block
			// Only set skip to false if no outer 'if' is currently skipping
			if len(*ifStk) > 0 {
				outerSkipState, err := ifStk.peek()
				if err != nil {
					return err
				}
				*skip = !outerSkipState // Revert to outer if's skip state
			} else {
				*skip = false // No outer if, so execute
			}
		}
		ifStk.push(!prevIfState) // Push the new state for potential nested 'else' or 'endif'
		return nil
	case "endif":
		if len(*ifStk) == 0 {
			return fmt.Errorf("endif without a preceding if")
		}
		_, err := ifStk.pop() // Pop from stack
		if err != nil {
			return err
		}
		if len(*ifStk) > 0 {
			currentIfState, err := ifStk.peek()
			if err != nil {
				return err
			}
			*skip = !currentIfState // Revert to parent if's skip state
		} else {
			*skip = false // No more if blocks, so no skipping
		}
		return nil
	}
	return nil
}

func handleOutputCommand(args string, outputFile *string) {
	*outputFile = args
}

func handleConcatCommand(args string, itemsToConcat *[]ConcatItem, baseDir string) {
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: true, Value: args, BaseDir: baseDir})
}

func handleIncludeCommand(args string, currentInstructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string) error {
	includePath := args
	if !filepath.IsAbs(includePath) {
		absPath, err := filepath.Abs(filepath.Join(filepath.Dir(currentInstructionsFile), includePath))
		if err != nil {
			return fmt.Errorf("error resolving absolute path for %s: %v", includePath, err)
		}
		includePath = absPath
	}
	err := processInstructions(includePath, outputFile, itemsToConcat, parameters, filepath.Dir(includePath))
	if err != nil {
		return err
	}
	return nil
}

func handleParamCommand(args string, parameters map[string]string) error {
	paramParts := strings.SplitN(args, "=", 2)
	if len(paramParts) == 2 {
		paramName := paramParts[0]
		paramValue := paramParts[1] // This is the value that needs substitution

		// Perform substitution on the value before storing it
		substitutedValue := substituteParams(paramValue, parameters)

		// 'param' has lower precedence than 'set'. Only set if not already defined.
		if _, exists := parameters[paramName]; !exists {
			parameters[paramName] = substitutedValue
		}
	} else {
		return fmt.Errorf("invalid param command format: %s", args)
	}
	return nil
}

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
		}
	} else {
		return fmt.Errorf("invalid set command format: %s", args)
	}
	return nil
}

func handlePrintCommand(args string, itemsToConcat *[]ConcatItem, parameters map[string]string) error {
	// Add the parameter reference itself, to be substituted in the final pass.
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: false, Value: fmt.Sprintf("${%s}", args)})
	return nil
}

func handleEmitCommand(args string, itemsToConcat *[]ConcatItem, parameters map[string]string) {
	// Defer substitution to the final pass to respect parameter precedence.
	*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: false, Value: args})
}

func dispatchCommand(line string, instructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string, currentPrefix *string, ifStk *ifStack, skip *bool) (bool, error) {
	textBegan := false // New variable to track if text-begin was found
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
		*currentPrefix = args
		return textBegan, nil
	}

	if *skip {
		return textBegan, nil
	}

	switch command {
	case "output":
		handleOutputCommand(args, outputFile)
	case "concat":
		handleConcatCommand(args, itemsToConcat, baseDir)
	case "include":
		return textBegan, handleIncludeCommand(args, instructionsFile, outputFile, itemsToConcat, parameters, baseDir)
	case "param":
		return textBegan, handleParamCommand(args, parameters)
	case "set":
		return textBegan, handleSetCommand(args, parameters)
	case "print":
		return textBegan, handlePrintCommand(args, itemsToConcat, parameters)
	case "emit":
		handleEmitCommand(args, itemsToConcat, parameters)
	case "text-begin":
		textBegan = true
	default:
		return textBegan, fmt.Errorf("unknown command: %s", command)
	}
	return textBegan, nil
}

func processInstructions(instructionsFile string, outputFile *string, itemsToConcat *[]ConcatItem, parameters map[string]string, baseDir string) error {
	file, err := os.Open(instructionsFile)
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

	for scanner.Scan() {
		line := scanner.Text()

		if inTextBlock {
			trimmedLine := strings.TrimSpace(line)
			if currentPrefix != "" {
				prefixWithColon := currentPrefix + ":"
				if strings.HasPrefix(trimmedLine, prefixWithColon) {
					trimmedLine = strings.TrimPrefix(trimmedLine, prefixWithColon)
				}
			}

			if trimmedLine == "text-end" {
				*itemsToConcat = append(*itemsToConcat, ConcatItem{IsFile: false, Value: textBlock.String()})
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

		textBegan, err := dispatchCommand(trimmedLine, instructionsFile, outputFile, itemsToConcat, parameters, baseDir, &currentPrefix, &ifStk, &skip)
		if err != nil {
			return err
		}
		inTextBlock = textBegan
	}

	if len(ifStk) > 0 {
		return fmt.Errorf("unclosed if block(s)")
	}

	return scanner.Err()
}

func runConcat(outputWriter io.Writer, itemsToConcat []ConcatItem, parameters map[string]string) error {
	for _, item := range itemsToConcat {
		// Unescape special characters just before writing.
		valueToWrite := unescapeString(item.Value)
		if item.IsFile {
			resolvedPath := valueToWrite
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
