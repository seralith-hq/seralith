# Gate: a compliant change passes, then a protected path with no approval fails.
Output docs/assets/demo/gate.gif

Require seralith
Require git

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

Type `seralith run --ws retry --agent worker --no-pty -- /bin/sh "$DEMO_BIN/write-retry.sh"` Enter
Sleep 1.4s

Type `seralith commit --ws retry -m "feat: add retry helper"` Enter
Sleep 1.8s

Type `seralith gate --range main..seralith/retry; echo "exit $?"` Enter
Sleep 2s

Type "seralith ws new auth" Enter
Sleep 1.2s

Type `seralith run --ws auth --agent worker --no-pty -- /bin/sh "$DEMO_BIN/write-token.sh"` Enter
Sleep 1.4s

Type `seralith commit --ws auth -m "feat: add token helper"` Enter
Sleep 2.2s

Type `seralith gate --range main..seralith/auth; echo "exit $?"` Enter
Sleep 2.6s
