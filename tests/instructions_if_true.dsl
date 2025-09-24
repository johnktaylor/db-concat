param ENV=dev
if ENV=dev
    concat ..\1.sql
else
    concat ..\2.sql
endif