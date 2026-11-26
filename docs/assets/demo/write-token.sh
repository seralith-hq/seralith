#!/bin/sh
# Write a file under internal/auth. The default policy requires an approval
# for a change on this path, so the gate rejects the commit until one exists.
mkdir -p internal/auth
printf 'package auth\n\nfunc Token() string { return "ok" }\n' > internal/auth/token.go
