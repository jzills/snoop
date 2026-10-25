# Rendered by demo/shots.sh: the site's screenshots, against the same
# fictional sessions as the demo plus three live ones. Never record against
# real sessions.

Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 24
Set FontFamily "Ubuntu Mono"
Set Theme {"background":"#0d1117","foreground":"#e6edf3","cursor":"#a99cff","black":"#0d1117","red":"#ff7b72","green":"#56d364","yellow":"#e3b341","blue":"#6cb6ff","magenta":"#f778ba","cyan":"#3fd1b0","white":"#e6edf3","brightBlack":"#7d8590","brightRed":"#ff7b72","brightGreen":"#56d364","brightYellow":"#e3b341","brightBlue":"#6cb6ff","brightMagenta":"#f778ba","brightCyan":"#3fd1b0","brightWhite":"#ffffff"}

Type "snoop"
Enter
Sleep 2s
Screenshot site/static/shots/picker.png
Sleep 300ms

Type "s"
Sleep 3s
Screenshot site/static/shots/summary.png
Sleep 300ms
Type "s"
Sleep 500ms

Type "f"
Sleep 500ms
Type "ratelimit"
Sleep 2s
Screenshot site/static/shots/find.png
Sleep 300ms
Escape
Sleep 500ms

# The rate-limiting session
Type "/"
Type "rate limit"
Enter
Enter
Sleep 2s
Screenshot site/static/shots/timeline.png
Sleep 300ms

Type "N"
Sleep 500ms
Type "N"
Sleep 1.5s
Screenshot site/static/shots/turn.png
Sleep 300ms

Type "X"
Sleep 1.5s
Screenshot site/static/shots/subagent.png
Sleep 300ms

Type "4"
Sleep 1s
Down
Sleep 500ms
Down
Sleep 1.5s
Screenshot site/static/shots/files.png
Sleep 300ms

Type "3"
Sleep 1.5s
Screenshot site/static/shots/stats.png
Sleep 300ms

Type "q"
Sleep 500ms
