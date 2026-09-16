#!/usr/bin/env python3
"""
BranchBase TUI Interactive Preview Runner
No database setup or Go compiler required!
"""

import os
import sys
import time

RESET = "\033[0m"
BOLD = "\033[1m"
DIM = "\033[2m"
GREEN = "\033[32m"
CYAN = "\033[36m"
YELLOW = "\033[33m"
RED = "\033[31m"
SELECTED_BG = "\033[44;97;1m"
CLEAR_SCREEN = "\033[2J\033[H"
HIDE_CURSOR = "\033[?25l"
SHOW_CURSOR = "\033[?25h"

class BranchInfo:
    def __init__(self, name, db, size_bytes, is_protected=False, is_active=False):
        self.name = name
        self.db = db
        self.size_bytes = size_bytes
        self.is_protected = is_protected
        self.is_active = is_active

def format_bytes(b):
    if b >= 1024 * 1024 * 1024:
        return f"{b / (1024**3):.1f} GB"
    elif b >= 1024 * 1024:
        return f"{b / (1024**2):.1f} MB"
    elif b >= 1024:
        return f"{b / 1024:.1f} KB"
    return f"{b} B"

def main():
    branches = [
        BranchInfo("main", "myapp_dev", 44564480, is_protected=True, is_active=False),
        BranchInfo("feat/tui-dashboard", "myapp_dev_feat_tui_dashboard", 44878848, is_active=True),
        BranchInfo("feature/stripe-billing", "myapp_dev_feature_stripe_billing", 45193216, is_active=False),
        BranchInfo("fix/error-packet", "myapp_dev_fix_error_packet", 44564480, is_active=False),
    ]

    selected_index = 1
    flash_msg = "Use ↑/↓ or j/k to navigate • Enter to switch • r to refresh • q to quit"
    flash_type = "info"
    active_branch = "feat/tui-dashboard"

    # Enable ANSI terminal on Windows
    if os.name == 'nt':
        os.system('')
        import msvcrt
    else:
        import tty, termios

    sys.stdout.write(HIDE_CURSOR)
    sys.stdout.flush()

    try:
        while True:
            # Render
            total_bytes = sum(b.size_bytes for b in branches)
            sanitized_active = active_branch.replace('/', '_').replace('-', '_')

            lines = []
            lines.append(f"{BOLD}{GREEN}🌿 BranchBase Dashboard{RESET} {DIM}v0.2.0{RESET}")
            lines.append(f"{DIM}─────────────────────────────────────────────────────────────────────────────{RESET}")
            lines.append(f"  • {BOLD}{'Active Branch:':<16}{RESET} {BOLD}{CYAN}{active_branch}{RESET} (sanitized: {sanitized_active})")
            lines.append(f"  • {BOLD}{'Database Engine:':<16}{RESET} {BOLD}SQLite / PostgreSQL{RESET} (127.0.0.1:5433)")
            lines.append(f"  • {'Proxy Routing:':<16} Port 5432 -> Backend 127.0.0.1:5433")
            lines.append(f"  • {'Managed Storage:':<16} {len(branches)} database(s) ({format_bytes(total_bytes)} total)")
            lines.append(f"{DIM}─────────────────────────────────────────────────────────────────────────────{RESET}\n")

            lines.append(f"{BOLD}  {'':<3} {'BRANCH':<24} {'DATABASE':<28} {'SIZE':<10} {'STATUS':<12}{RESET}")
            lines.append(f"{DIM}  {'':<3} {'------':<24} {'--------':<28} {'----':<10} {'------':<12}{RESET}")

            for i, b in enumerate(branches):
                is_sel = (i == selected_index)
                cursor = f"{BOLD}{CYAN}▶ {RESET}" if is_sel else "  "
                prefix = "*" if b.is_active else " "
                
                status_text = "Idle"
                status_color = DIM
                if b.is_protected:
                    status_text = "Protected"
                    status_color = YELLOW
                if b.is_active:
                    status_text = "Active"
                    status_color = f"{BOLD}{GREEN}"

                row_str = f"{prefix:<2} {b.name:<24} {b.db:<28} {format_bytes(b.size_bytes):<10} {status_color}{status_text:<12}{RESET}"
                if is_sel:
                    lines.append(f"{cursor}{SELECTED_BG} {row_str} {RESET}")
                else:
                    lines.append(f"{cursor} {row_str}")

            lines.append(f"\n{DIM}─────────────────────────────────────────────────────────────────────────────{RESET}")
            
            f_color = CYAN
            if flash_type == "success":
                f_color = GREEN
            elif flash_type == "error":
                f_color = RED

            lines.append(f"{BOLD}{f_color}  ℹ {flash_msg}{RESET}")
            lines.append(f"{DIM}  [↑/k] Up  [↓/j] Down  [Enter/s] Switch  [r] Refresh  [q] Quit{RESET}\n")

            sys.stdout.write(CLEAR_SCREEN + "\n".join(lines))
            sys.stdout.flush()

            # Input handling
            if os.name == 'nt':
                ch = msvcrt.getch()
                if ch in (b'\x00', b'\xe0'): # Arrow keys
                    arrow = msvcrt.getch()
                    if arrow == b'H': # Up
                        if selected_index > 0:
                            selected_index -= 1
                    elif arrow == b'P': # Down
                        if selected_index < len(branches) - 1:
                            selected_index += 1
                    continue
                
                try:
                    char = ch.decode('latin1', errors='ignore').lower()
                except:
                    continue

                if char in ('q', '\x1b', '\x03'):
                    break
                elif char in ('k', 'w'):
                    if selected_index > 0:
                        selected_index -= 1
                elif char in ('j', 's'):
                    if selected_index < len(branches) - 1:
                        selected_index += 1
                elif char == 'r':
                    flash_msg = "Refreshed databases and storage metrics successfully"
                    flash_type = "success"
                elif char in ('\r', '\n'):
                    sel = branches[selected_index]
                    for b in branches:
                        b.is_active = False
                    sel.is_active = True
                    active_branch = sel.name
                    flash_msg = f"Switched active database target to '{sel.db}'"
                    flash_type = "success"

            else:
                # Unix fallback
                fd = sys.stdin.fileno()
                old_settings = termios.tcgetattr(fd)
                try:
                    tty.setraw(sys.stdin.fileno())
                    ch = sys.stdin.read(1)
                finally:
                    termios.tcsetattr(fd, termios.TCSADRAIN, old_settings)

                if ch.lower() == 'q':
                    break
                elif ch == '\x1b':
                    seq = sys.stdin.read(2)
                    if seq == '[A' and selected_index > 0:
                        selected_index -= 1
                    elif seq == '[B' and selected_index < len(branches) - 1:
                        selected_index += 1

    finally:
        sys.stdout.write(SHOW_CURSOR + "\n" + RESET)
        sys.stdout.flush()
        print("TUI closed gracefully.")

if __name__ == "__main__":
    main()
