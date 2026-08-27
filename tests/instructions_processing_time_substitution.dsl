param VALUE=one
emit ${VALUE}@@n
text-begin
${VALUE}
text-end
print VALUE
emit @@n
param FILE=..\1.sql
concat ${FILE}
emit @@n
set VALUE=two
set FILE=..\2.sql
emit ${VALUE}@@n
text-begin
${VALUE}
text-end
print VALUE
emit @@n
concat ${FILE}
emit @@n
