#!/bin/sh
# Append one function to retry.go. The tape runs this inside a workspace
# session so the new lines are attributed to that session's actor.
printf '\nfunc Delay() int { return 10 }\n' >> retry.go
