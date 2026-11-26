# Terminal demos

Two recordings of the `seralith` binary, made with
[VHS](https://github.com/charmbracelet/vhs) tapes. Each tape sources
`setup.sh`, which creates a throwaway git repository and a new signing key,
then runs real commands. Nothing in the GIFs is mocked or edited by hand.

| Tape | GIF | What it shows |
| --- | --- | --- |
| `authorship.tape` | `authorship.gif` | A workspace worktree, a manual edit, a session, the signed change attestation stored in `refs/notes/seralith`, and that note read back |
| `gate.tape` | `gate.gif` | `seralith gate` passing on a compliant change, then failing because a protected path has no approval |

## Re-render

From the repository root, with `vhs`, `ttyd`, and `ffmpeg` on `PATH`:

```sh
make demo
```

`make demo` builds `bin/seralith` from this checkout and runs both tapes with
that binary. Keys and repositories are created under `/tmp` at record time.
Commit the new GIFs when command output changes.
