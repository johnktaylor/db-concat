# db-concat

A simple tool to concatenate SQL files based on a set of instructions.

## Building

To build the script, run the following command:

```bash
go build -o db-concat.exe
```

This will create an executable file named `db-concat.exe` (on Windows) or `db-concat` (on macOS/Linux).

## Running the Application

To run the script, you need to create an instruction file (e.g., `instructions.dsl`) and then run the following command:

```bash
./db-concat [OPTIONS] <instructions_file>
```

**Options:**

*   `--param-file <filename>`: Comma-separated list of parameter files (key=value per line). Parameters loaded from these files have the lowest precedence.
*   `--param <key>=<value>`: Key-value pair parameter. Can be specified multiple times. These parameters have the highest precedence, overriding both parameter files and DSL `param` commands.
*   `--output <filename>`: Specifies the output file path. If not specified, output goes to `stdout`. This is overridden by the `output` DSL command.

## DSL Commands

The following commands are available in the instruction file:

*   `output <filename>`: Specifies the output file for the concatenation. This overrides any `--output` command-line flag.
*   `concat <filename>`: Adds a SQL file to the list of files to be concatenated. File paths can be relative to the instruction file. This command does not add a newline after the file content. To add a newline, use the `emit` command with the `@@n` special character (e.g., `emit @@n`).
*   `include <filename>`: Includes another instruction file. Paths can be relative to the current instruction file.
*   `text-begin`: Starts a block of inline text.
*   `text-end`: Ends a block of inline text.
*   `param <key>=<value>`: Defines a parameter within the instruction file. These parameters override values from `--param-file` but are overridden by `--param` command-line arguments.
*   `if <condition>`: Starts a conditional block. The block is executed if the condition is true.
    *   **Condition Format:** `KEY=VALUE`. Compares the value of a parameter `KEY` with `VALUE`.
    *   Also supports numerical comparisons: `KEY>VALUE`, `KEY>=VALUE`, `KEY<VALUE`, `KEY<=VALUE`.
*   `else`: Executes the following block if the preceding `if` condition was false.
*   `endif`: Ends a conditional block.
*   `print <param_name>`: Outputs the value of the specified parameter to the output stream.
*   `emit <text>`: Outputs a string of text directly into the concatenated output stream. This command does not automatically add a newline character. To add a newline, use the `@@n` special character. It also supports `@@r` (carriage return), `@@t` (tab), and `@@s` (space).
*   `set <param_name>=<value>`: Assigns a new value to a parameter. The value can be a literal string or contain parameter substitutions (e.g., `set KEY=${ANOTHER_VAR}`).
*   `param <key>=<value>`: Defines a parameter within the instruction file. This command will only set the parameter if it has not already been defined by a command-line `--param` flag or a DSL `set` command. It overrides values from `--param-file`. The `<value>` part of the command supports parameter substitution (e.g., `param MY_VAR=${EXISTING_VAR}`).
*   `if <condition>`: Starts a conditional block. The block is executed if the condition is true.
    *   **Condition Format:** `KEY=VALUE`. Compares the value of a parameter `KEY` with `VALUE`.
    *   Also supports numerical comparisons: `KEY>VALUE`, `KEY>=VALUE`, `KEY<VALUE`, `KEY<=VALUE`.
*   `else`: Executes the following block if the preceding `if` condition was false.
*   `endif`: Ends a conditional block.
*   `print <param_name>`: Outputs the value of the specified parameter to the output stream.
*   `emit <text>`: Outputs a string of text directly into the concatenated output stream. This command does not automatically add a newline character. To add a newline, use the `@@n` special character. It also supports `@@r` (carriage return), `@@t` (tab), and `@@s` (space).
*   `set <param_name>=<value>`: Assigns a new value to a parameter. This command overrides parameters from `--param-file` and DSL `param` commands. However, it **cannot** override a parameter that has been set by a command-line `--param` flag (which has the highest precedence). The `<value>` part of the command supports parameter substitution (e.g., `set KEY=${ANOTHER_VAR}`).
*   `set-prefix <prefix>`: Sets a mandatory prefix for all subsequent commands in the current file. Unprefixed commands will be ignored.
*   `clear-prefix`: When prefixed (e.g., `<prefix>:clear-prefix`), this command removes the active prefix requirement for the rest of the file.

## Parameter Handling

Parameters can be defined and overridden at different levels, with the following precedence (highest to lowest):

1.  **Command-line `--param` flags:** These have the absolute highest precedence. A parameter set via a `--param` flag cannot be overridden by any DSL command (`param` or `set`).
2.  **DSL `set` commands:** These assign a new value to a parameter. They override parameters from `--param-file` and DSL `param` commands, but are themselves overridden by command-line `--param` flags.
3.  **DSL `param` commands:** These define a parameter, but only if it hasn't already been defined by a higher-precedence source (i.e., command-line `--param` or a DSL `set` command). They override parameters loaded from `--param-file`.
4.  **`--param-file`:** Parameters loaded from specified files have the lowest precedence.

**Parameter Substitution:**
Parameters can be used within DSL command arguments using the `${KEY}` syntax (e.g., `concat ${MY_FILE}.sql`, `emit Hello ${MY_VAR}`). Importantly, `param` and `set` commands also perform parameter substitution on their assigned values (e.g., `set KEY=${ANOTHER_VAR}`) at the time the command is processed.

## Conditional Logic

The `if`, `else`, and `endif` commands allow for conditional execution of DSL instructions.

*   An `if` block starts with `if <condition>` and ends with `endif`.
*   An optional `else` command can be used to define a block that executes if the `if` condition is false.
*   Conditions are currently limited to `KEY=VALUE` comparisons, where `KEY` is a parameter name and `VALUE` is the string to compare against.
*   Numerical comparisons (`>`, `>=`, `<`, `<=`) are also supported. For these, both values are treated as numbers. If conversion to a number fails, the condition is false.

## Outputting Variables

The `print <param_name>` command can be used to output the value of a defined parameter directly into the concatenated output stream. This is useful for embedding dynamic information or for debugging.

## Command Prefixes

The `set-prefix` and `clear-prefix` commands allow you to scope commands within a specific file.

- `set-prefix <prefix>`: After this command, all subsequent commands in the same file must be prefixed with `<prefix>:` to be executed. Unprefixed commands are ignored.
- `<prefix>:clear-prefix`: This command removes the prefix requirement.

The prefix is strictly file-scoped and does not affect `include`d files.

**Example:**
```dsl
# commands.dsl
concat file1.sql      # Executed
set-prefix my-app
concat file2.sql      # Ignored
my-app:concat file3.sql # Executed
my-app:clear-prefix
concat file4.sql      # Executed
```

## Example Instruction File

```dsl
# Instructions for the DSL

output concatenated.sql

param ENVIRONMENT=dev

if ENVIRONMENT=dev
    text-begin
    -- Development specific SQL
    text-end
    concat 1.sql
else
    text-begin
    -- Production specific SQL
    text-end
    concat 2.sql
endif

print ENVIRONMENT

include include_instructions.dsl

text-begin
-- Another comment
text-end

concat 2.sql
```

## Running Tests

To run the automated test suite, navigate to the `tests` directory and run the following command:

```bash
go run run_tests.go
```

The script will build the `db-concat` executable, run all defined test cases, and report the results.