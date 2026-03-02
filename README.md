# mnm (Make No Mistake)

**A smart, drop-in wrapper for dangerous terminal commands that adds a universal `undo` button.**

Tired of accidentally deleting the wrong file with `rm` or overwriting a config file in `nano`? `mnm` acts as a safety net for your daily terminal operations. It transparently intercepts commands, creates intelligent backups, and allows you to revert the state of your files with a simple `mnm undo`.

## Features

* **Instant O(1) Backups:** Uses filesystem hardlinks for non-destructive commands (`rm`, `cp`, `mv`). It takes zero extra disk space and zero milliseconds to backup.
* **Smart In-Place Fallback:** Automatically detects commands that modify files in-place (like `nano`, `vim`, or `sed -i`) and safely falls back to physical O(N) copies.
* **Orphan Cleanup:** If a command creates new files (e.g., the destination file in `cp a.txt b.txt`), `mnm undo` will automatically track and delete the newly created "orphan" file.
* **Concurrency Safe:** Implements an OS-level atomic spin-lock (`history.lock`) so multiple background scripts using `mnm` won't corrupt your undo history.
* **Native Exit Codes:** Perfectly propagates standard I/O and exit codes back to the OS. If a wrapped command fails, `mnm` fails with the exact same exit code, making it safe for shell scripting.

## Installation

Ensure you have [Go](https://go.dev/) installed, then run:

```bash
go install github.com/Targothh/mnm@latest
```

## Usage

Use mnm just like you would use the standard commands.
Bash

# Delete a file safely
$ mnm rm important_document.txt

# Copy a file safely
$ mnm cp old.config new.config

# Edit a file safely
$ mnm nano server.conf

The Magic Button

Made a mistake? Just type:
Bash

$ mnm undo
Recovering files from: rm important_document.txt
File recovered: /home/user/important_document.txt

## Supported Commands

By default, mnm protects:
rm, cp, mv, nano, vim, vi, pico, sed (dynamically detects -i), perl (dynamically detects -i).
Advanced: Shell Aliases

For maximum safety, you can alias your default commands in your .bashrc or .zshrc so you don't even have to type mnm:
Bash

alias rm="mnm rm"
alias cp="mnm cp"
alias mv="mnm mv"

## How It Works (Under the Hood)

mnm keeps a lightweight JSON history stack in ~/.mnm_history/.
When evaluating a command, it parses the arguments to differentiate between existing targets and newly created destinations. Depending on the target binary's behavior, it chooses either an O(1) hardlink strategy or a physical file copy to preserve the exact inode state before execution. The history stack is automatically capped at 15 entries to prevent disk clutter.
License

This project is licensed under the MIT License - see the LICENSE file for details.
