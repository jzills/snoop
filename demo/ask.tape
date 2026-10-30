# Rendered by demo/shots.sh: the live board with a session asking for
# permission, against the demo's three live sessions. Taller than
# live.tape, to fit the request under them.
# Never record against real sessions.

Set FontSize 16
Set Width 960
Set Height 460
Set Padding 24
Set FontFamily "Ubuntu Mono"
Set Theme {"background":"#0d1117","foreground":"#e6edf3","cursor":"#a99cff","black":"#0d1117","red":"#ff7b72","green":"#56d364","yellow":"#e3b341","blue":"#6cb6ff","magenta":"#f778ba","cyan":"#3fd1b0","white":"#e6edf3","brightBlack":"#7d8590","brightRed":"#ff7b72","brightGreen":"#56d364","brightYellow":"#e3b341","brightBlue":"#6cb6ff","brightMagenta":"#f778ba","brightCyan":"#3fd1b0","brightWhite":"#ffffff"}

Type "snoop --live"
Enter
Sleep 3s
Screenshot site/static/shots/ask.png
Sleep 300ms
Type "q"
Sleep 500ms
