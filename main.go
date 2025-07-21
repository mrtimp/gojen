package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/jessevdk/go-flags"
	"github.com/manifoldco/promptui"
	log "github.com/sirupsen/logrus"
)

type Options struct {
	Project     string `short:"p" long:"project" description:"Project name"`
	TemplateDir string `long:"template-dir" description:"Optional template directory (helper for developing locally)"`
}

const (
	TemplateRepo   = "https://github.com/mrtimp/gojen"
	TemplateSubDir = "template"
)

func main() {
	var opts Options
	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(1)
	}

	projectName := promptString("Project name", opts.Project)
	destinationDir := promptString("Destination directory (default: ./"+projectName+")", opts.Project)

	if destinationDir == "" {
		destinationDir = "./" + projectName
	}

	if _, err := os.Stat(destinationDir); err == nil {
		log.Fatalf("Destination directory '%s' already exists.", destinationDir)
	} else if !os.IsNotExist(err) {
		log.Fatalf("Error with the destination directory: %v", err)
	}

	templateRepo := promptString("Template repo (HTTPS URL)", TemplateRepo)
	repoSubPath := promptString("Subdirectory inside repo to copy (optional)", TemplateSubDir)

	useGoFlags := promptBool("Should this project use go-flags for parsing CLI arguments?", 0)
	useLogRus := promptBool("Should this project use the Logrus for structured logging?", 0)
	useAWSSDK := promptBool("Should this project use the AWS SDK V2?", 0)

	buildTargets := promptMultiSelect("Select build targets", []string{"Linux", "Darwin", "Windows"})
	buildArchitectures := promptMultiSelect("Select build architectures", []string{"AMD64", "ARM64"})

	runGitInit := promptBool("Shall I run git init", 0)

	tempDir, err := os.MkdirTemp("", "gojen")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	log.Infoln("Cloning template...")
	if opts.TemplateDir == "" {
		if err := cloneRepo(templateRepo, tempDir); err != nil {
			log.Fatal(err)
		}
	} else {
		tempDir = opts.TemplateDir
	}

	sourcePath := filepath.Join(tempDir, repoSubPath)
	log.Infoln("Copying and templating files...")
	if err := copyAndTemplate(sourcePath, destinationDir, map[string]any{
		"PROJECT_NAME": projectName,

		"USE_AWS":     useAWSSDK,
		"USE_GOGLAGS": useGoFlags,
		"USE_LOGRUS":  useLogRus,

		"BUILD_TARGETS":       buildTargets,
		"BUILD_ARCHITECTURES": buildArchitectures,
	}); err != nil {
		log.Fatal(err)
	}

	if runGitInit {
		log.Infoln("Initializing Git...")
		if err := gitInit(destinationDir); err != nil {
			log.Fatal(err)
		}
	}

	if err := goModTidy(destinationDir); err != nil {
		log.Infoln("Installed Go modules...")
		log.Warn(err)
	}

	log.Infoln("Project bootstrapped at:", destinationDir)
}

func promptString(label string, placeHolder string) string {
	prompt := promptui.Prompt{
		Label:   label,
		Default: placeHolder,
	}
	result, _ := prompt.Run()

	return result
}

func promptBool(label string, cursorPos int) bool {
	prompt := promptui.Select{
		Label:     label,
		Items:     []string{"Yes", "No"},
		CursorPos: cursorPos,
	}
	idx, _, _ := prompt.Run()

	return idx == 0
}

func promptMultiSelect(label string, items []string) []string {
	var selected []string
	for _, item := range items {
		confirmed := promptBool(fmt.Sprintf("Build for %s?", item), 0)
		if confirmed {
			selected = append(selected, item)
		}
	}
	return selected
}

func cloneRepo(repoURL, destPath string) error {
	cmd := exec.Command("git", "clone", "--depth=1", repoURL, destPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func gitInit(path string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func goModTidy(path string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func copyAndTemplate(src, dst string, data map[string]any) error {
	templateFunction := template.FuncMap{
		"lower": strings.ToLower,
	}
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if strings.HasSuffix(info.Name(), ".tmpl") {
			target = filepath.Join(dst, strings.TrimSuffix(rel, ".tmpl"))

			tmpl, err := template.New(info.Name()).Funcs(templateFunction).Parse(string(content))
			if err != nil {
				return fmt.Errorf("template parse error in %s: %w", path, err)
			}

			var rendered bytes.Buffer
			if err := tmpl.Execute(&rendered, data); err != nil {
				return fmt.Errorf("template execution error in %s: %w", path, err)
			}

			return os.WriteFile(target, rendered.Bytes(), info.Mode())
		}

		if err := os.MkdirAll(filepath.Dir(target), os.ModePerm); err != nil {
			return err
		}
		return os.WriteFile(target, content, info.Mode())
	})
}
