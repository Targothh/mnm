package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type BackupStrategy int

const (
	StrategyNone BackupStrategy = iota
	HardLink
	Copy
)

var supported_commands = map[string]BackupStrategy{
	"rm":   HardLink,
	"cp":   HardLink,
	"mv":   HardLink,
	"vim":  Copy,
	"vi":   Copy,
	"nano": Copy,
}

type FileBackup struct {
	OriginalPath string `json:"original"`
	BackupPath   string `json:"backup"`
}

type CommandLog struct {
	Command      string       `json:"command"`
	Files        []FileBackup `json:"files"`
	CreatedFiles []string     `json:"created_files"`
}

func choose_strategy(cmd_name string, args []string) BackupStrategy {
	if cmd_name == "sed" || cmd_name == "perl" {
		for _, arg := range args {
			if strings.HasPrefix(arg, "-i") || arg == "--in-place" {
				return Copy
			}
		}
		return StrategyNone
	}

	if strategy, exists := supported_commands[cmd_name]; exists {
		return strategy
	}

	return StrategyNone
}

/*
func for creating a backup (O(1) hardlink with O(N) copy fallback)
*/
func smart_backup(src_path string, backup_dir string, strategy BackupStrategy) (string, error) {
	if strategy == StrategyNone {
		return "", nil
	}

	file_name := filepath.Base(src_path)
	backup_path := filepath.Join(backup_dir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), file_name))

	if strategy == HardLink {
		err := os.Link(src_path, backup_path)
		if err == nil {
			return backup_path, nil
		}
	}

	src, err := os.Open(src_path)
	if err != nil {
		return "", err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return "", err
	}

	dst, err := os.OpenFile(backup_path, os.O_CREATE|os.O_WRONLY, info.Mode())
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	return backup_path, nil
}

/*
func that parses and returns both existing paths and potential new paths (orphans)
*/
func parse_paths(args []string) ([]string, []string) {
	var existing []string
	var missing []string

	for _, arg := range args {
		// skips args with "-" on start
		if strings.HasPrefix(arg, "-") {
			continue
		}

		abs_path, err := filepath.Abs(arg)
		if err != nil {
			continue
		}

		info, err := os.Stat(abs_path)
		if err == nil && !info.IsDir() {
			existing = append(existing, abs_path)
		} else if os.IsNotExist(err) {
			missing = append(missing, abs_path)
		}
	}

	return existing, missing
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("  mnm <command> [args...]")
		fmt.Println("  mnm undo")
		return
	}

	command := os.Args[1]

	home_dir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Home path not found: (%v)\n", err)
		os.Exit(1)
	}
	backup_dir := filepath.Join(home_dir, ".mnm_history")
	history_file := filepath.Join(backup_dir, "history.json")
	lock_file := filepath.Join(backup_dir, "history.lock")

	// file lock
	for {
		f, err := os.OpenFile(lock_file, os.O_CREATE|os.O_EXCL, 0666)
		if err == nil {
			f.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer os.Remove(lock_file)

	if _, err := os.Stat(backup_dir); os.IsNotExist(err) {
		os.MkdirAll(backup_dir, os.ModePerm)
	}

	// undo implementation
	if command == "undo" {
		data, err := os.ReadFile(history_file)
		if err != nil {
			fmt.Println("Nothing to recover")
			return
		}

		var history []CommandLog
		if err := json.Unmarshal(data, &history); err != nil || len(history) == 0 {
			fmt.Println("Stack is empty, nothing to recover")
			return
		}

		last_idx := len(history) - 1
		last_cmd := history[last_idx]

		fmt.Printf("Recovering file before: %s\n", last_cmd.Command)

		for _, file := range last_cmd.Files {
			if _, err := os.Stat(file.BackupPath); os.IsNotExist(err) {
				fmt.Printf("Backup file missing: %s\n", file.OriginalPath)
				continue
			}
			target_dir := filepath.Dir(file.OriginalPath)
			if err := os.MkdirAll(target_dir, os.ModePerm); err != nil {
				fmt.Printf("Error while mkdir %s: %v\n", target_dir, err)
				continue
			}

			err = os.Rename(file.BackupPath, file.OriginalPath)
			if err != nil {
				fmt.Printf("Recovery failed %s: %v\n", file.OriginalPath, err)
			} else {
				fmt.Printf("File recovered: %s\n", file.OriginalPath)
			}
		}

		for _, orphan := range last_cmd.CreatedFiles {
			if _, err := os.Stat(orphan); err == nil {
				os.Remove(orphan)
				fmt.Printf("File removed (orphan): %s\n", orphan)
			}
		}

		history = history[:last_idx]

		if len(history) > 0 {
			new_data, _ := json.MarshalIndent(history, "", "  ")
			os.WriteFile(history_file, new_data, 0644)
		} else {
			os.Remove(history_file)
		}
		return
	}

	// wrapper for commands
	cmd_name := command
	cmd_args := os.Args[2:]

	strategy := choose_strategy(cmd_name, cmd_args)

	if strategy != StrategyNone {
		var backed_up_files []FileBackup

		existing_files, missing_files := parse_paths(cmd_args)

		for _, abs_path := range existing_files {
			backup_path, backup_err := smart_backup(abs_path, backup_dir, strategy)

			if backup_err == nil && backup_path != "" {
				backed_up_files = append(backed_up_files, FileBackup{
					OriginalPath: abs_path,
					BackupPath:   backup_path,
				})
			}
		}

		if len(backed_up_files) > 0 || len(missing_files) > 0 {
			var history []CommandLog
			data, err := os.ReadFile(history_file)
			if err == nil {
				json.Unmarshal(data, &history)
			}

			full_command := cmd_name + " " + strings.Join(cmd_args, " ")
			new_entry := CommandLog{
				Command:      full_command,
				Files:        backed_up_files,
				CreatedFiles: missing_files,
			}

			history = append(history, new_entry)

			// limiting history for 15 commands
			if len(history) > 15 {
				oldest_cmd := history[0]
				for _, f := range oldest_cmd.Files {
					os.Remove(f.BackupPath)
				}
				history = history[1:]
			}

			new_data, _ := json.MarshalIndent(history, "", "  ")
			os.WriteFile(history_file, new_data, 0644)
		}
	}

	cmd := exec.Command(cmd_name, cmd_args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	err = cmd.Run()

	if err != nil {
		os.Remove(lock_file)

		// propagate errror code
		if exitError, ok := err.(*exec.ExitError); ok {
			os.Exit(exitError.ExitCode())
		}
		// error if binary does not exist
		fmt.Fprintf(os.Stderr, "Error command not found: %v\n", err)
		os.Exit(1)
	}

}
