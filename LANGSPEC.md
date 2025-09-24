# DSL Language Specification for db-concat

This document provides a detailed specification for the Domain Specific Language (DSL) used by the `db-concat` Go application. This DSL allows users to define a sequence of operations for concatenating SQL files, embedding text, and managing parameters, with support for conditional logic.

## 1. Introduction

The `db-concat` tool is designed to combine multiple SQL files and inline text into a single output file. The order and content of the concatenation are controlled by an instruction file written in this DSL. The language is line-oriented, with each significant line representing a command.

## 2. General Syntax Rules

*   **Line-Oriented:** Each command must reside on its own line.
*   **Comments:** Lines starting with `#` are treated as comments and are ignored by the parser.
*   **Whitespace:** Leading and trailing whitespace on a line is trimmed before parsing the command and its arguments.
*   **Case-Sensitivity:** Commands are case-sensitive (e.g., `concat` is recognized, `CONCAT` is not).
*   **Parameter Substitution:** Parameters can be referenced within command arguments using the `${KEY}` syntax. These will be substituted with their current values during processing.

## 3. Commands

### 3.1 `output <filename>`

*   **Purpose:** Specifies the path for the final concatenated output file.
*   **Arguments:**
    *   `<filename>`: The path to the output file. This can be an absolute or relative path. Relative paths are resolved against the directory of the instruction file.
*   **Behavior:** If this command is used, it overrides any `--output` command-line flag. If no `output` command is specified and no `--output` flag is provided, the output is written to `stdout`.
*   **Example:**
    ```dsl
    output ./build/final_schema.sql
    ```

### 3.2 `concat <filename>`

*   **Purpose:** Adds a SQL file to the list of files to be concatenated.
*   **Arguments:**
    *   `<filename>`: The path to the SQL file. This can be an absolute or relative path. Relative paths are resolved against the directory of the instruction file.
*   **Behavior:** The content of the specified file will be included in the final output at the point this command is processed in the instruction sequence. The file content is included as-is, without any additional newlines. To add a newline after the file, use the `emit` command (e.g., `emit @@n`).
*   **Example:**
    ```dsl
    concat ../common/setup.sql
    concat tables/users.sql
    ```

### 3.3 `include <filename>`

*   **Purpose:** Includes and processes another DSL instruction file.
*   **Arguments:**
    *   `<filename>`: The path to another DSL instruction file. This can be an absolute or relative path. Relative paths are resolved against the directory of the *current* instruction file.
*   **Behavior:** The `db-concat` tool will pause processing the current file, process all commands in the included file, and then resume processing the current file from where it left off. Parameters defined in the included file will affect the current file and vice-versa.
*   **Example:**
    ```dsl
    include common_instructions.dsl
    ```

### 3.4 `text-begin` / `text-end`

*   **Purpose:** Defines a block of inline text to be included directly in the output.
*   **Arguments:** None for both commands.
*   **Behavior:** All lines between `text-begin` and `text-end` (exclusive) will be treated as literal text and appended to the output. Each line within the block will have a newline character (\n) appended to it. Parameter substitution *does* occur within `text-begin`/`text-end` blocks.
*   **Note:** Parameter substitution happens when the final output is generated, not when the text block is parsed.
*   **Example:**
    ```dsl
    text-begin
    -- This is an inline SQL comment.
    INSERT INTO settings (key, value) VALUES ('version', '${DB_VERSION}');
    text-end
    ```

### 3.5 `param <key>=<value>`

*   **Purpose:** Defines a parameter within the instruction file.
*   **Arguments:**
    *   `<key>`: The name of the parameter.
    *   `<value>`: The value to assign to the parameter. This value *does* undergo parameter substitution at the time of definition.
*   **Behavior:** This command will only set the parameter if it has not already been defined by a command-line `--param` flag or a DSL `set` command. It overrides values from `--param-file`.
*   **Example:**
    ```dsl
    param DB_VERSION=1.0.0
    param FULL_VERSION=${DB_VERSION}-beta
    ```

### 3.6 `set <key>=<value>`

*   **Purpose:** Assigns a new value to an existing parameter or defines a new one.
*   **Arguments:**
    *   `<key>`: The name of the parameter.
    *   `<value>`: The value to assign to the parameter. This value *does* undergo parameter substitution at the time of assignment, allowing for dynamic values based on other parameters.
*   **Behavior:** This command overrides parameters from `--param-file` and DSL `param` commands. However, it **cannot** override a parameter that has been set by a command-line `--param` flag (which has the highest precedence).
*   **Example:**
    ```dsl
    param SCHEMA_NAME=public
    set FULL_TABLE_NAME=${SCHEMA_NAME}.users
    ```

### 3.7 `print <param_name>`

*   **Purpose:** Outputs the value of a specified parameter directly into the concatenated output stream.
*   **Arguments:**
    *   `<param_name>`: The name of the parameter whose value should be printed.
*   **Behavior:** The current value of the parameter will be written to the output. This is useful for embedding dynamic information or for debugging.
*   **Example:**
    ```dsl
    print CURRENT_SCHEMA
    ```

### 3.8 `emit <text>`

*   **Purpose:** Outputs a string of text directly into the concatenated output stream.
*   **Arguments:**
    *   `<text>`: The string to be outputted.
*   **Behavior:** This command does not automatically add a newline character. To add a newline, use the `@@n` special character. Parameter substitution (`${KEY}`) occurs within `<text>`. Special escape sequences `@@n` (newline), `@@r` (carriage return), `@@t` (tab), and `@@s` (space) are interpreted and converted to their respective characters.
*   **Example:**
    ```dsl
    emit This is a line with a new line@@nand a tab@@tcharacter and a space@@scharacter.@@n
    ```

### 3.9 `if <condition>` / `else` / `endif`

*   **Purpose:** Provides conditional execution of DSL instructions.
*   **Arguments for `if`:**
    *   `<condition>`: A condition in the format `KEY=VALUE`. The block following the `if` will be executed if the parameter `KEY` has an exact string match with `VALUE`.
    *   Also supports numerical comparisons: `KEY>VALUE`, `KEY>=VALUE`, `KEY<VALUE`, `KEY<=VALUE`. For these, both `KEY`'s value and `VALUE` are parsed as numbers. If either is not a valid number, the condition is false.
*   **Arguments for `else` / `endif`:** None.
*   **Behavior:**
    *   An `if` block starts with `if <condition>` and ends with `endif`.
    *   An optional `else` command can be used to define a block that executes if the preceding `if` condition was false.
    *   `if` blocks can be nested.
*   **Example:**
    ```dsl
    if ENVIRONMENT=production
    concat deploy/production_fixes.sql
    else
    concat deploy/dev_data.sql
    endif

    param DB_VERSION=2.5
    if DB_VERSION>2.0
      concat migrations/v3_migration.sql
    endif
    ```

### 3.10 `set-prefix <prefix>`

*   **Purpose:** Sets a mandatory prefix for all subsequent commands within the current DSL file.
*   **Arguments:**
    *   `<prefix>`: The string to be used as a prefix.
*   **Behavior:** See Section 4, "Command Scoping with Prefixes."

### 3.11 `clear-prefix`

*   **Purpose:** Removes the mandatory prefix requirement from the point it is called.
*   **Arguments:** None.
*   **Behavior:** This command must itself be prefixed (e.g., `<prefix>:clear-prefix`). See Section 4, "Command Scoping with Prefixes."

## 4. Command Scoping with Prefixes

The DSL provides a mechanism to namespace or scope commands within a single file using prefixes. This can be useful to avoid unintended command execution in complex DSL files or to create logical groups of commands.

### `set-prefix <prefix>`

When the `set-prefix` command is used, the DSL parser enters a "prefixed" mode for the current file. From that point on, any subsequent command will only be recognized and executed if it is explicitly prefixed with `<prefix>:`. Commands that are not prefixed will be ignored.

### `<prefix>:clear-prefix`

To exit the "prefixed" mode and return to normal command processing, the `clear-prefix` command must be used. Crucially, this command must also be prefixed with the currently active prefix.

### Scope

The prefix scope is strictly limited to the file in which `set-prefix` was called. When the parser begins processing a new file (e.g., via the `include` command), it starts in a non-prefixed state. Once the included file is fully processed, the parser restores the prefix state of the parent file.

### Example

Consider the following two files:

`main.dsl`:
```dsl
# This command will be executed
concat file1.sql

# Set a prefix for this file
set-prefix my-app

# This command is now ignored because it lacks the prefix
concat file2.sql

# This command will be executed
my-app:concat file3.sql

# This will include and process other.dsl normally
my-app:include other.dsl

# The prefix is still active after the include
my-app:concat file4.sql

# Clear the prefix
my-app:clear-prefix

# This command will be executed
concat file5.sql
```

`other.dsl`:
```dsl
# The prefix from main.dsl does not apply here.
# This command will be executed.
concat other_file.sql
```

## 5. Parameter Handling and Precedence

Parameters are key-value pairs that can be used to store dynamic information. They can be defined and overridden at different levels, with a clear precedence:

1.  **Command-line `--param <key>=<value>` flags (Highest Precedence):** These parameters are passed directly when running `db-concat`. A parameter set via a `--param` flag cannot be overridden by any DSL command (`param` or `set`).
2.  **DSL `set <key>=<value>` commands:** These commands within the instruction file assign a new value to a parameter. They override parameters defined by `param` commands or loaded from `--param-file`. Values assigned via `set` undergo parameter substitution at the time of assignment.
3.  **DSL `param <key>=<value>` commands:** These commands within the instruction file define parameters. They will only set the parameter if it has not already been defined by a command-line `--param` flag or a DSL `set` command. Their values undergo parameter substitution at the time of definition.
4.  **`--param-file <filename>` (Lowest Precedence):** Parameters loaded from external files (one `key=value` pair per line) have the lowest precedence and are overridden by all other methods.

**Parameter Substitution:** When a parameter is referenced using `${KEY}` syntax (e.g., `concat ${MY_FILE}.sql`), the tool will replace `${KEY}` with the current value of `MY_FILE` from its internal parameter map. This substitution occurs for arguments of `concat`, `include`, `output`, `set` (for the value being assigned), `emit`, and within `text-begin`/`text-end` blocks.

## 5. Error Handling

The `db-concat` tool provides informative error messages for common issues:

*   **Unknown Command:** If an unrecognized command is encountered in a DSL file.
*   **Invalid Command Format:** If a command's arguments do not match the expected format (e.g., `param` without an `=`).
*   **Unclosed If Block:** If an `if` command is not matched by an `endif`.
*   **Else Without If:** If an `else` command is encountered without a preceding `if`.
*   **Parameter Not Found:** If a `print` command references a parameter that has not been defined.
*   **File Not Found:** If `concat` or `include` commands reference files that do not exist.

## 7. Example DSL File

```dsl
# Example DSL Instruction File

# Define a default schema name
param DEFAULT_SCHEMA=app_schema

# Set the output file, using a parameter for the version
param DB_VERSION=1.0.0
output ./build/schema_v${DB_VERSION}.sql

# Include common setup instructions
include common/base_setup.dsl

# Conditionally include environment-specific data
if ENVIRONMENT=development
  concat data/dev_users.sql
  set CURRENT_SCHEMA=dev_schema # Override schema for development
else
  concat data/prod_users.sql
  set CURRENT_SCHEMA=${DEFAULT_SCHEMA} # Use default for production
endif

# Print the current schema being used
text-begin
-- Current Schema: ${CURRENT_SCHEMA}
text-end

# Concatenate core tables
concat tables/products.sql
concat tables/orders.sql

# Inline text for a stored procedure
text-begin
CREATE OR REPLACE FUNCTION calculate_total(order_id INT) RETURNS DECIMAL AS $$
BEGIN
    RETURN (SELECT SUM(price * quantity) FROM order_items WHERE order_id = $1);
END;
$$
LANGUAGE plpgsql;
text-end

# Another conditional block based on a different parameter
param FEATURE_FLAG=true
if FEATURE_FLAG=true
  concat features/new_feature_tables.sql
  print FEATURE_FLAG
endif

# Final cleanup script
concat cleanup.sql
```
