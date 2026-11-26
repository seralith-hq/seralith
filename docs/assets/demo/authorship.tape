# Authorship tracking: a worktree edit, a session, a signed attestation in
# refs/notes/seralith, and that note read back.
Output docs/assets/demo/authorship.gif

Require seralith
Require git
Require python3

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 20
Set Width 1280
Set Height 640
Set Padding 18
Set Framerate 12
Set TypingSpeed 8ms
Set CursorBlink false
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set WindowBarSize 28

Hide
Type "source docs/assets/demo/setup.sh" Enter
Show

Type "seralith ws new retry" Enter
Sleep 1.2s

Type "ws=$(seralith ws path retry)" Enter
Sleep 400ms

Type `printf 'package retry\n\nfunc Attempts() int { return 3 }\n' > "$ws/retry.go"` Enter
Sleep 500ms

Type `seralith run --ws retry --agent worker --no-pty -- /bin/sh "$DEMO_BIN/append-delay.sh"` Enter
Sleep 1.4s

Type "seralith status --ws retry" Enter
Sleep 2s

Type `seralith commit --ws retry -m "feat: add retry helper"` Enter
Sleep 2s

Type `git notes --ref=refs/notes/seralith show seralith/retry | "$DEMO_BIN/show-note"` Enter
Sleep 2.4s

Type "seralith blame retry.go --rev seralith/retry" Enter
Sleep 2.6s
