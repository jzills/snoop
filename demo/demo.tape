# Rendered by demo/render.sh, which builds snoop, seeds fictional sessions
# and points CLAUDE_CONFIG_DIR at them. Never record against real sessions.
Output demo/demo.gif

Set FontSize 15
Set Width 1400
Set Height 820
Set FontFamily "Ubuntu Mono"
Set Framerate 24
Set TypingSpeed 60ms
Set Theme {"background":"#0d1117","foreground":"#e6edf3","cursor":"#a99cff","black":"#0d1117","red":"#ff7b72","green":"#56d364","yellow":"#e3b341","blue":"#6cb6ff","magenta":"#f778ba","cyan":"#3fd1b0","white":"#e6edf3","brightBlack":"#7d8590","brightRed":"#ff7b72","brightGreen":"#56d364","brightYellow":"#e3b341","brightBlue":"#6cb6ff","brightMagenta":"#f778ba","brightCyan":"#3fd1b0","brightWhite":"#ffffff"}

Type "snoop"
Enter
Sleep 2.5s

# Totals across every session
Type "s"
Sleep 3.5s
Down 12
Sleep 2s
Type "s"
Sleep 1s

# Open the newest session: the context meter and per-prompt totals
Enter
Sleep 2s

# A prompt's turn: totals, slowest calls, waterfall
Type "N"
Sleep 1s
Type "N"
Sleep 3.5s

# Expand the subagent inline
Type "X"
Sleep 2.5s

# Every file the session touched, with its diffs
Type "4"
Sleep 2s
Down
Sleep 1.5s
Down
Sleep 2.5s

# Stats: tokens, cost, context and the Turns breakdown
Type "3"
Sleep 2s
Down 14
Sleep 3s

Type "q"
Sleep 500ms
