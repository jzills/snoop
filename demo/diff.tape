# Rendered by demo/shots.sh: the Diff tab's screenshot, against a fictional
# repository and the session that changed it. Never record against real
# sessions.

Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 24
Set FontFamily "Ubuntu Mono"
Set Theme {"background":"#0d1117","foreground":"#e6edf3","cursor":"#a99cff","black":"#0d1117","red":"#ff7b72","green":"#56d364","yellow":"#e3b341","blue":"#6cb6ff","magenta":"#f778ba","cyan":"#3fd1b0","white":"#e6edf3","brightBlack":"#7d8590","brightRed":"#ff7b72","brightGreen":"#56d364","brightYellow":"#e3b341","brightBlue":"#6cb6ff","brightMagenta":"#f778ba","brightCyan":"#3fd1b0","brightWhite":"#ffffff"}

Type "snoop"
Enter
Sleep 2s
Type "/"
Type "Retry-After"
Enter
Enter
Sleep 2s

Type "5"
Sleep 1.5s
Down
Sleep 300ms
Tab
Sleep 300ms
Down@100ms 15
Sleep 300ms
Type "c"
Sleep 300ms
Type "This rounds down, so a 0.4s wait says Retry-After: 0. Round up with math.Ceil."
Enter
Sleep 1s
Screenshot site/static/shots/diff.png
Sleep 300ms

Type "q"
Sleep 300ms
Type "d"
Sleep 500ms
Type "q"
Sleep 500ms
