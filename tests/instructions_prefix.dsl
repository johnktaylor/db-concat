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
