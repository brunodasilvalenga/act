package doctor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brunodasilvalenga/act/internal/config"
)

// fixLogPath returns the path to the doctor fix log, alongside ~/.act.json.
func fixLogPath() string {
	cfgPath := config.ConfigPath()
	if cfgPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cfgPath), ".act-doctor-fix.log")
}

// runFixes walks results in order, and for every statusFail result with a
// non-nil Fix, prompts for confirmation (unless skipConfirm is true), runs
// the fix, and appends a timestamped record to the fix log. It returns the
// updated results slice (re-running the originating check after a
// successful fix, so the final report reflects the post-fix state) and
// whether every attempted fix succeeded.
func runFixes(results []result, skipConfirm bool, recheck func(name string) result) ([]result, bool) {
	logPath := fixLogPath()
	var logFile *os.File
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err == nil {
			logFile = f
			defer logFile.Close()
		}
	}

	reader := bufio.NewReader(os.Stdin)
	allOK := true

	for i, r := range results {
		if r.Status != statusFail || r.Fix == nil {
			continue
		}

		desc := r.Fix.Describe()
		fmt.Printf("\n%s: %s\n", r.Name, r.Detail)
		fmt.Printf("Proposed fix: %s\n", desc)

		if !skipConfirm {
			fmt.Print("Proceed? [y/N]: ")
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Println("Skipped.")
				logLine(logFile, r.Name, "SKIPPED (user declined)")
				continue
			}
		}

		var writers []io.Writer = []io.Writer{os.Stdout}
		if logFile != nil {
			writers = append(writers, logFile)
		}
		w := io.MultiWriter(writers...)

		fmt.Fprintf(w, "[%s] Fixing %s...\n", timestamp(), r.Name)
		err := r.Fix.Apply(w)
		if err != nil {
			fmt.Fprintf(w, "[%s] FAILED: %v\n", timestamp(), err)
			allOK = false
			continue
		}
		fmt.Fprintf(w, "[%s] Done.\n", timestamp())

		if recheck != nil {
			results[i] = recheck(r.Name)
		}
	}

	if logPath != "" {
		fmt.Printf("\nFix log written to %s\n", logPath)
	}
	return results, allOK
}

func logLine(f *os.File, name, msg string) {
	if f == nil {
		return
	}
	fmt.Fprintf(f, "[%s] %s: %s\n", timestamp(), name, msg)
}

func timestamp() string {
	return time.Now().Format(time.RFC3339)
}
