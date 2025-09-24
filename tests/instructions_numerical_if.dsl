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