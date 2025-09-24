param OUTER=true
param INNER=true

if OUTER=true
    concat ../1.sql
    emit @@n
    if INNER=true
        concat ../2.sql
        emit @@n
    else
        concat ../3.sql
        emit @@n
    endif
    concat ../4.sql
    emit @@n
else
    concat ../5.sql
    emit @@n
endif

text-begin
-- After outer block
text-end