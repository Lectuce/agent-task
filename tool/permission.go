package tool

import (
	"agent/config"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

var DenyList = []string{"rm -rf /", "sudo", "shutdown", "reboot", "mkfs", "dd if=", "> /dev/sda"}

type permissionRule struct {
	tools   []string
	check   func(map[string]any) bool
	message string
}

var permissionRules = []permissionRule{
	{
		tools: []string{"write_file", "edit_file"},
		check: func(args map[string]any) bool {
			path := args["path"].(string)
			fullPath := filepath.Join(config.WORKDIR, path)

			absPath, err := filepath.Abs(fullPath)
			if err != nil {
				return true
			}

			absWorkdir, err := filepath.Abs(config.WORKDIR)
			if err != nil {
				return true
			}

			rel, err := filepath.Rel(absWorkdir, absPath)
			if err != nil {
				return true
			}

			// 如果路径跳出了 WORKDIR
			return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
		},
		message: "Writing outside workspace",
	},
	{
		tools: []string{"bash"},
		check: func(args map[string]any) bool {
			command, _ := args["command"].(string)

			dangerous := []string{
				"rm ",
				"> /etc/",
				"chmod 777",
			}

			for _, kw := range dangerous {
				if strings.Contains(command, kw) {
					return true
				}
			}

			return false
		},
		message: "Potentially destructive command",
	},
}

func CheckDenyList(command string) error {
	for i := range DenyList {
		if strings.Contains(command, DenyList[i]) {
			return fmt.Errorf("Blocked: '%v' is on the deny list", DenyList[i])
		}
	}
	return nil
}

func CheckRules(toolName string, args map[string]any) string {
	for _, rule := range permissionRules {
		if slices.Contains(rule.tools, toolName) && rule.check(args) {
			return rule.message
		}
	}
	return ""
}

func AskUser(toolName string, args map[string]any, reason string) string {
	fmt.Printf("⚠  %v", reason)
	fmt.Printf("Tool: %v({%#v})", toolName, args)
	var choice string
	fmt.Println("   Allow? [y/N] ")
	fmt.Scan(&choice)
	if choice == "y" || choice == "yes" {
		return "allow"
	} else {
		return "deny"
	}
}
