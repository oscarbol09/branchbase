# VHS Tape for BranchBase Terminal Demo
# Generate GIF with: vhs assets/demo.tape

Output docs/assets/demo.gif
Output assets/demo.gif

Set FontSize 16
Set Width 1000
Set Height 550
Set Theme "Dracula"
Set Padding 20

Type "branchbase init"
Sleep 500ms
Enter
Sleep 1500ms

Type "git checkout -b feature/stripe-billing"
Sleep 500ms
Enter
Sleep 1500ms

Type "npx prisma migrate dev"
Sleep 500ms
Enter
Sleep 2000ms

Type "git checkout main"
Sleep 500ms
Enter
Sleep 1500ms

Type "branchbase tui"
Sleep 500ms
Enter
Sleep 3000ms
