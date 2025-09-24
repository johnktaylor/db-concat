param ENV=prod
if ENV=dev
    concat ..\1.sql
else
    concat ..\2.sql
endif