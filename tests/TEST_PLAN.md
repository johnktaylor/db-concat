# Test Plan for db-concat

This plan documents the automated integration cases in `tests/run_tests.go`, which are executed by Go's `TestIntegrationSuite` in `tests/run_tests_test.go`. Each case builds `db-concat`, invokes it with the listed fixture category, and checks the stated result. File-content assertions compare the generated result with the corresponding `expected_*` fixture unless noted otherwise.

## Running the Suite

Run the suite from the project root:

```bash
go test ./...
```

The Go test invokes the runner from the project root. The runner writes its normal fixture outputs, captures stdout or stderr where required, checks expected failures, and removes only its explicitly listed transient artifacts.

## Test Cases

| # | Case | Fixtures and invocation | Expected result |
| --- | --- | --- | --- |
| 1 | Parameter files | `instructions_param_file.dsl` with `--param-file params.txt` | File parameters substitute into command arguments. |
| 2 | DSL `param` overrides parameter file | `instructions_param_overrides_file.dsl` with `--param-file params_param_override.txt` | The DSL value is emitted. |
| 3 | Command-line parameters | `instructions_cli_param.dsl` with `--param CLI_VAR=1` | CLI parameter substitutes into the concat path. |
| 4 | Invalid command-line parameter | `--param INVALID` | Fails with `Invalid --param value`. |
| 5 | Whitespace-only command-line parameter key | `--param "   =value"` | Fails with `Invalid --param value`. |
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

## Coverage Maintenance

`tests/run_tests.go` is the source of truth for integration cases, and `tests/run_tests_test.go` exposes them to standard Go tooling. When a case is added, removed, or changed, update this table in the same change so that the case count, scenario, and expected result remain accurate.
