# Test Plan for db-concat

This document outlines the automated test suite for the `db-concat` tool, detailing the purpose, input, execution, and expected output for each test case.

## How to Run Tests

To execute the test suite, navigate to the project's root directory and run:

```bash
go run tests/run_tests.go
```

## Test Cases

### Test 1: Parameter Files (`--param-file`)

*   **Purpose:** Verifies that parameters can be loaded from an external file and used for substitution in DSL commands.
*   **Input Files:**
    *   `tests/params.txt`:
        ```
        MY_VAR=1
        ```
    *   `tests/instructions_param_file.dsl`:
        ```dsl
        param ANOTHER_VAR=2
        concat ${MY_VAR}.sql
        concat ${ANOTHER_VAR}.sql
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --param-file tests\params.txt --output tests\output_param_file.sql tests\instructions_param_file.dsl
    ```
*   **Expected Output:** `tests/output_param_file.sql` should contain `SELECT 1;SELECT 2;`

### Test 2: Command-line Parameters (`--param`)

*   **Purpose:** Verifies that parameters can be passed directly via command-line arguments and used for substitution.
*   **Input Files:**
    *   `tests/instructions_cli_param.dsl`:
        ```dsl
        concat ${CLI_VAR}.sql
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --param CLI_VAR=1 --output tests\output_cli_param.sql tests\instructions_cli_param.dsl
    ```
*   **Expected Output:** `tests/output_cli_param.sql` should contain `SELECT 1;`

### Test 3: DSL `param` Command

*   **Purpose:** Verifies that parameters can be defined directly within the DSL instruction file, including support for parameter substitution within the assigned value.
*   **Input Files:**
    *   `tests/instructions_dsl_param.dsl`:
        ```dsl
        param DSL_VAR=2
        concat ${DSL_VAR}.sql
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_dsl_param.sql tests\instructions_dsl_param.dsl
    ```
*   **Expected Output:** `tests/output_dsl_param.sql` should contain `SELECT 2;`

### Test 4: Parameter Precedence (CLI > DSL > File)

*   **Purpose:** Verifies that command-line parameters have the highest precedence, followed by DSL-defined parameters, and then parameters from files.
*   **Input Files:**
    *   `tests/params_precedence.txt`:
        ```
        OVERRIDE_VAR=FromFile
        ```
    *   `tests/instructions_precedence.dsl`:
        ```dsl
        param OVERRIDE_VAR=FromDSL
        concat ${OVERRIDE_VAR}.sql
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --param-file tests\params_precedence.txt --param OVERRIDE_VAR=1 --output tests\output_precedence.sql tests\instructions_precedence.dsl
    ```
*   **Expected Output:** `tests/output_precedence.sql` should contain `SELECT 1;`

### Test 5a: `if` Condition is True

*   **Purpose:** Verifies that the `if` block is executed when its condition evaluates to true.
*   **Input Files:**
    *   `tests/instructions_if_true.dsl`:
        ```dsl
        param ENV=dev
        if ENV=dev
            concat 1.sql
        else
            concat 2.sql
        endif
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_if_true.sql tests\instructions_if_true.dsl
    ```
*   **Expected Output:** `tests/output_if_true.sql` should contain `SELECT 1;`

### Test 5b: `if` Condition is False

*   **Purpose:** Verifies that the `else` block is executed when the `if` condition evaluates to false.
*   **Input Files:**
    *   `tests/instructions_if_false.dsl`:
        ```dsl
        param ENV=prod
        if ENV=dev
            concat 1.sql
        else
            concat 2.sql
        endif
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_if_false.sql tests\instructions_if_false.dsl
    ```
*   **Expected Output:** `tests/output_if_false.sql` should contain `SELECT 2;`

### Test 6: `print` Command

*   **Purpose:** Verifies that the `print` command outputs the value of a specified parameter to the output stream.
*   **Input Files:**
    *   `tests/instructions_print.dsl`:
        ```dsl
        param MESSAGE=HelloFromPrint
        print MESSAGE
        concat 1.sql
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_print.sql tests\instructions_print.dsl
    ```
*   **Expected Output:** `tests/output_print.sql` should contain `HelloFromPrintSELECT 1;`

### Test 7a: Output to `stdout`

*   **Purpose:** Verifies that the tool outputs to standard output when no output file is specified.
*   **Input Files:**
    *   `tests/instructions_output.dsl`:
        ```dsl
        concat 1.sql
        ```
*   **Command:**
    ```bash
    .\db-concat.exe tests\instructions_output.dsl
    ```
*   **Expected Output:** `stdout` should contain `SELECT 1;`

### Test 7b: Output to File Using `--output` Flag

*   **Purpose:** Verifies that the tool outputs to a specified file when the `--output` flag is used.
*   **Input Files:**
    *   `tests/instructions_output.dsl` (same as 7a)
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_file.sql tests\instructions_output.dsl
    ```
*   **Expected Output:** `tests/output_file.sql` should contain `SELECT 1;`

### Test 8a: Unclosed `if` Block Error Handling

*   **Purpose:** Verifies that the tool correctly reports an error for unclosed `if` blocks.
*   **Input Files:**
    *   `tests/instructions_unclosed_if.dsl`:
        ```dsl
        if ENV=dev
            concat 1.sql
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_error_unclosed_if.sql tests\instructions_unclosed_if.dsl
    ```
*   **Expected Output:** `stderr` should contain `Error processing instructions: unclosed if block(s)` and the command should exit with a non-zero status.

### Test 8b: Unknown Command Error Handling

*   **Purpose:** Verifies that the tool correctly reports an error for unknown DSL commands.
*   **Input Files:**
    *   `tests/instructions_unknown_command.dsl`:
        ```dsl
        unknown_cmd arg
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_error_unknown_command.sql tests\instructions_unknown_command.dsl
    ```
*   **Expected Output:** `stderr` should contain `Error processing instructions: unknown command: unknown_cmd` and the command should exit with a non-zero status.

### Test 9: `set` Command

*   **Purpose:** Verifies that the `set` command correctly assigns values to parameters, including using parameter substitution.
*   **Input Files:**
    *   `tests/instructions_set.dsl`:
        ```dsl
        param INITIAL_VAR=InitialValue
        set NEW_VAR=LiteralValue
        set TRANSFORMED_VAR=${INITIAL_VAR}_Transformed
        print NEW_VAR
        print TRANSFORMED_VAR
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_set.sql tests\instructions_set.dsl
    ```
*   **Expected Output:** `tests/output_set.sql` should contain `LiteralValueInitialValue_Transformed`

### Test 10: `emit` Command

*   **Purpose:** Verifies that the `emit` command outputs the specified text, including parameter substitution and special character unescaping, without automatically adding a newline.
*   **Input Files:**
    *   `tests/instructions_emit.dsl`:
        ```dsl
        param MY_VAR=World
        emit Hello, ${MY_VAR}!@@nThis is a new line.And a carriage return.@@rAnd a tab.@@t
        emit Another line.
        emit A line with@@sspaces.
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_emit.sql tests\instructions_emit.dsl
    ```
*   **Expected Output:** `tests/output_emit.sql` should contain `Hello, World!\nThis is a new line.And a carriage return.\rAnd a tab.\tAnother line.A line with spaces.`

### Test 11: Prefix Commands (`set-prefix`, `clear-prefix`)

*   **Purpose:** Verifies that `set-prefix` correctly scopes commands and `clear-prefix` removes the scope.
*   **Input Files:**
    *   `tests/instructions_prefix.dsl`:
        ```dsl
        # This should be included
        concat ..\1.sql

        set-prefix myapp

        # This should be ignored
        concat ..\2.sql

        # This should be included
        myapp:concat ..\2.sql

        # This should clear the prefix
        myapp:clear-prefix

        # This should be included again
        concat ..\1.sql
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_prefix.sql tests\instructions_prefix.dsl
    ```
*   **Expected Output:** `tests/output_prefix.sql` should contain `SELECT 1;SELECT 2;SELECT 1;`

### Test 12: Nested `if` Statements

*   **Purpose:** Verifies that nested `if` and `else` blocks are correctly processed.
*   **Input Files:**
    *   `tests/instructions_nested_if.dsl`:
        ```dsl
        param OUTER=true
        param INNER=false

        if OUTER=true
            concat 1.sql
            if INNER=true
                concat 2.sql
            else
                concat 3.sql
            endif
            concat 4.sql
        else
            concat 5.sql
        endif
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_nested_if.sql tests\instructions_nested_if.dsl
    ```
*   **Expected Output:** `tests/output_nested_if.sql` should contain `SELECT 1;SELECT 3;SELECT 4;`

### Test 13: Numerical `if` Conditions

*   **Purpose:** Verifies that numerical comparison operators (`>`, `>=`, `<`, `<=`) work correctly in `if` conditions.
*   **Input Files:**
    *   `tests/instructions_numerical_if.dsl`:
        ```dsl
        param VERSION=3.5
        param COUNT=10

        # Test > (true)
        if VERSION>3.0
            emit GT_TRUE
        endif

        # Test < (false)
        if VERSION<3.0
            emit LT_FALSE
        endif

        # Test >= (true)
        if COUNT>=10
            emit GTE_TRUE
        endif

        # Test <= (false)
        if COUNT<=9
            emit LTE_FALSE
        endif

        # Test with non-numeric (false)
        if VERSION>abc
            emit NON_NUMERIC
        endif
        ```
*   **Command:**
    ```bash
    .\db-concat.exe --output tests\output_numerical_if.sql tests\instructions_numerical_if.dsl
    ```
*   **Expected Output:** `tests/output_numerical_if.sql` should contain `GT_TRUEGTE_TRUE`