# Test Plan for db-concat

This plan documents the native Go tests in `db-concat_test.go`. Each scenario is an independently selectable test that invokes the application in-process and checks the stated result. File-content assertions compare temporary generated results with the corresponding read-only `expected_*` fixture unless noted otherwise. Fixture paths use portable forward slashes so the suite behaves consistently on Windows and Linux.

## Running the Suite

Run the suite from the project root:

```bash
go test ./...
```

The tests use `testing.T.TempDir` for generated files, so running the suite does not modify repository fixtures or require a cleanup pass.

## Native Go Tests

`db-concat_test.go` exercises the callable CLI entry point in-process. The tests are individually selectable with `go test -run <TestName>` and contribute directly to code coverage and race detection.

## Test Cases

| # | Case | Fixtures and invocation | Expected result |
| --- | --- | --- | --- |
| 1 | Parameter files | `instructions_param_file.dsl` with `--param-file params.txt` | File parameters substitute into command arguments. |
| 2 | DSL `param` overrides parameter file | `instructions_param_overrides_file.dsl` with `--param-file params_param_override.txt` | The DSL value is emitted. |
| 3 | Command-line parameters | `instructions_cli_param.dsl` with `--param CLI_VAR=1` | CLI parameter substitutes into the concat path. |
| 4 | Invalid command-line parameter | `--param INVALID` | Fails with `invalid --param value`. |
| 5 | Whitespace-only command-line parameter key | `--param "   =value"` | Fails with `invalid --param value`. |
| 6 | Non-recursive substitution | `instructions_non_recursive_substitution.dsl` with `FIRST=${SECOND}` and `SECOND=expanded` | Outputs the literal `${SECOND}` introduced by `FIRST`. |
| 7 | Empty DSL `param` key | `instructions_invalid_param_empty_key.dsl` | Fails with `invalid param command format`. |
| 8 | Empty DSL `set` key | `instructions_invalid_set_empty_key.dsl` | Fails with `invalid set command format`. |
| 9 | Empty parameter-file key | `--param-file params_invalid_empty_key.txt` | Fails with `invalid parameter file line format`. |
| 10 | DSL `param` command | `instructions_dsl_param.dsl` | A DSL parameter substitutes into a concat path. |
| 11 | Parameter precedence: CLI > DSL > file | `instructions_precedence.dsl`, `params_precedence.txt`, and CLI override | The CLI value is used. |
| 12 | True `if` condition | `instructions_if_true.dsl` | Only the true branch is concatenated. |
| 13 | False `if` condition | `instructions_if_false.dsl` | Only the `else` branch is concatenated. |
| 14 | `print` command | `instructions_print.dsl` | The current parameter value is written to output. |
| 15 | stdout output | `instructions_output.dsl` without `--output` | stdout matches `expected_output_stdout.txt`. |
| 16 | `--output` file output | `instructions_output.dsl` with `--output` | Output file matches the expected content and uses mode `0644` on Unix. |
| 17 | CLI output precedence | `instructions_output_precedence.dsl` with `--output` | CLI destination is used instead of the DSL `output` destination. |
| 18 | Unclosed `if` block | `instructions_unclosed_if.dsl` | Fails with `unclosed if block(s)`. |
| 19 | Failure preserves existing output | `instructions_output_failure.dsl` with a seeded destination | Fails to open input and leaves the destination unchanged. |
| 20 | Unclosed text block | `instructions_unclosed_text_block.dsl` | Fails with `unclosed text block`. |
| 21 | Unknown command | `instructions_unknown_command.dsl` | Fails with `unknown command`. |
| 22 | `set` command | `instructions_set.dsl` | Assigns and prints parameter values. |
| 23 | Precedence: `set` > `param` | `instructions_set_vs_param.dsl` | `set` value wins over DSL `param`. |
| 24 | Precedence: CLI > `set` | `instructions_cli_vs_set.dsl` with CLI parameter | CLI value wins over `set`. |
| 25 | `emit` command | `instructions_emit.dsl` | Emits text and decodes supported `@@` escapes. |
| 26 | Literal `@@` in concat path | `instructions_concat_literal_atat.dsl` | Opens the literal filename `source@@n.sql`; path escapes are not decoded. |
| 27 | Prefix commands | `instructions_prefix.dsl` | Executes only matching prefixed commands until `clear-prefix`. |
| 28 | Nested conditionals | `instructions_nested_if.dsl` | Selects the correct nested branches. |
| 29 | Numeric conditions | `instructions_numerical_if.dsl` | Supports numeric comparisons and treats non-numeric comparisons as false. |
| 30 | Inactive branch does not change prefix | `instructions_inactive_prefix.dsl` | Inactive commands do not affect active prefix state. |
| 31 | Inactive text block | `instructions_inactive_text_block.dsl` | Text in an inactive branch is discarded, including control-like text. |
| 32 | Processing-time substitution | `instructions_processing_time_substitution.dsl` | Each command uses parameter values available when it is processed. |
| 33 | Include-path substitution | `instructions_include_substitution.dsl` | Parameters substitute in an `include` path. |
| 34 | Missing parameter in `print` | `instructions_print_missing.dsl` | Fails with `parameter not found`. |
| 35 | Relative DSL output path | `relative_output/instructions_relative_output.dsl` | Resolves the DSL output path relative to its instruction file. |
| 36 | Prefixed `text-end` | `instructions_prefix_text_block.dsl` | Requires the active prefix on `text-end`. |
| 37 | Invalid `output` syntax | `instructions_invalid_output.dsl` | Fails with `invalid output command format`. |
| 38 | Invalid `concat` syntax | `instructions_invalid_concat.dsl` | Fails with `invalid concat command format`. |
| 39 | Invalid `include` syntax | `instructions_invalid_include.dsl` | Fails with `invalid include command format`. |
| 40 | Invalid `print` syntax | `instructions_invalid_print.dsl` | Fails with `invalid print command format`. |
| 41 | Invalid `if` syntax | `instructions_invalid_if.dsl` | Fails with `invalid if command format`. |
| 42 | Invalid `else` syntax | `instructions_invalid_else.dsl` | Fails with `invalid else command format`. |
| 43 | Duplicate `else` | `instructions_duplicate_else.dsl` | Fails with `duplicate else for if block`. |
| 44 | Include cycle | `instructions_include_cycle.dsl` | Fails with `include cycle detected`. |
| 45 | Invalid `endif` syntax | `instructions_invalid_endif.dsl` | Fails with `invalid endif command format`. |
| 46 | Invalid `text-begin` syntax | `instructions_invalid_text_begin.dsl` | Fails with `invalid text-begin command format`. |
| 47 | Invalid `set-prefix` syntax | `instructions_invalid_set_prefix.dsl` | Fails with `invalid set-prefix command format`. |
| 48 | Help | `--help` | Prints usage and succeeds. |
| 49 | Invalid flag | An unknown command-line flag | Reports the flag error once and fails. |
| 50 | Parameter file whitespace and empty entries | `--param-file` with spaces and empty comma entries | Skips empty entries and trims whitespace around file paths. |
| 51 | Stdout buffering on failure | `instructions_output_failure.dsl` in stdout mode | Leaves standard output empty when mid-run concatenation fails. |
| 52 | Literal `@@` escape (`@@@@`) | DSL with `@@@@` escape sequences in emit, print, and text block | Decodes `@@@@` to literal `@@` before other escape substitutions. |
| 53 | Condition whitespace, `!=` operator, and empty key validation | DSL with spaces around operators, `!=`, empty key, and missing key | Trims key and value, supports `!=`, treats missing key as false, and rejects empty key. |
| 54 | Handlers reject empty arguments | Calling `handleOutputCommand` and `handleConcatCommand` with empty string | Returns error instead of panicking. |
| 55 | Command arguments whitespace trimming | DSL with extra spaces between command and argument | Trims argument leading and trailing whitespace consistently. |
| 56 | File and line error context in DSL and includes | Parent DSL including child DSL with an unknown command | Reports errors in `file:line:` format and wraps include hierarchy. |
| 57 | Scanner error checked before unclosed block | DSL with >1 MiB line inside a `text-begin` block | Surfaces `token too long` error with file and line rather than `unclosed text block`. |
| 58 | Error wrapping sentinel checks | DSL including a nonexistent file | Preserves sentinel errors through wrapped error chains so `errors.Is(err, os.ErrNotExist)` succeeds. |
| 59 | Multiple parameter files with overlapping keys | `--param-file` with multiple comma-separated files | Later file values overwrite earlier files for overlapping keys while preserving unique keys. |
| 60 | Prefix scoping across includes | Prefixed parent DSL including child DSL | Child DSL runs unprefixed by default, and parent prefix is restored upon returning from include. |
| 61 | Child-to-parent parameter propagation | Parent DSL including child DSL defining/setting parameters | Parameters created or modified in the child via `param` and `set` are retained and visible in the parent. |
| 62 | `print` command escape decoding | DSL with `print` command targeting parameter containing `@@` escape sequences | Decodes `@@` sequences (`@@n`, `@@t`, `@@s`, `@@@@`) in parameter value when writing output. |
| 63 | Missing include file error context | DSL including a nonexistent file | Reports failure with parent file and line context, include target name, and preserves `os.ErrNotExist`. |
| 64 | Diamond include | DSL including the same file sequentially and across multiple include branches | Successfully processes each inclusion without falsely triggering cycle detection. |
| 65 | Top-level line length limit | Top-level line exceeding 1 MiB | Fails with scanner `token too long` error reporting file and line number. |
| 66 | Nonexistent parameter file | `--param-file` pointing to a nonexistent file | Fails with error reporting missing file and preserving `os.ErrNotExist`. |
| 67 | Nonexistent output directory | `--output` targeting a nonexistent directory | Fails with error creating temporary output file and preserving `os.ErrNotExist`. |
| 68 | Output overwrite preserves permissions | `--output` targeting an existing file | Overwrites output file with new content while preserving pre-existing file permissions. |
| 69 | Bare `emit` without arguments | DSL containing `emit` with no arguments | Emits an empty string without error. |

## Coverage Maintenance

`db-concat_test.go` is the source of truth for automated cases. When a test is added, removed, or changed, update this table in the same change so that the case count, scenario, and expected result remain accurate.
