#!/bin/sh
# Write a small source file. The gate tape runs this in a workspace session
# on an ordinary path, which the default policy allows.
printf 'package retry\n\nfunc Attempts() int { return 3 }\n' > retry.go
