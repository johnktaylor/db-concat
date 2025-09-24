# Test that 'set' overrides 'param'
param PRECEDENCE_VAR=from_param
set PRECEDENCE_VAR=from_set
print PRECEDENCE_VAR