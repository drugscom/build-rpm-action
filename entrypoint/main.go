package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/sethvargo/go-githubactions"
)


func buildPackage(spec string) error {
	if _, err := os.Stat(spec); err != nil {
		return err
	}

	// Assume spec is inside a SPECS directory in the build root
	buildDir := path.Dir(path.Dir(spec))

	// rpmbuild expects an absolute path
	if !path.IsAbs(buildDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		buildDir = path.Join(cwd, buildDir)
	}

	parsedSpec, err := parseSpec(spec)
	if err != nil {
		return err
	}

	if err := lintSpec(parsedSpec, path.Join(buildDir, ".rpmlintrc")); err != nil {
		return err
	}

	if err := installBuildDeps(parsedSpec); err != nil {
		return err
	}

	if err := downloadSources(parsedSpec, path.Join(buildDir, "SOURCES")); err != nil {
		return err
	}

	cmd := exec.Command("rpmbuild","-ba", "--nocheck", spec,
		"--define", fmt.Sprintf("_topdir %s", buildDir),
		"--define", "_build_name_fmt %%{NAME}-%%{VERSION}-%%{RELEASE}.%%{ARCH}.rpm",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func downloadSources(spec, dest string) error {
	githubactions.Group("Downloading sources")
	defer githubactions.EndGroup()

	if err := os.MkdirAll(dest,0755); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}


	cmd := exec.Command("spectool", "-g", "-C", dest, spec)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	return nil
}

func installBuildDeps(spec string) error {
	githubactions.Group("Installing build dependencies")
	defer githubactions.EndGroup()

	cmd := exec.Command("yum-builddep", "-y", spec)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	return nil
}

func installExtraPackages(packages... string) error {
	githubactions.Group("Installing extra packages")
	defer githubactions.EndGroup()

	cmdArgs := append([]string{"-y", "install"}, packages...)

	cmd := exec.Command("yum", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func lintSpec(spec, configFile string) error {
	githubactions.Group("Linting spec file")
	githubactions.AddMatcher(".github/rpmlint-problem-matcher.json")
	defer githubactions.RemoveMatcher("rpmlint")
	defer githubactions.EndGroup()

	cmdArgs := []string{spec}
	if configFile != "" {
		cmdArgs = append([]string{"-f", configFile}, cmdArgs...)
	}

	cmd := exec.Command("rpmlint", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	return nil
}

func parseSpec(spec string) (string, error) {
	githubactions.Group("Parsing spec file")
	defer githubactions.EndGroup()

	tempSpec, err := os.CreateTemp("", "*.spec")
	if err != nil {
		githubactions.Errorf(err.Error())
		return "", err
	}

	cmd := exec.Command("rpmspec", "-P", spec)
	cmd.Stdout = io.MultiWriter(os.Stdout, tempSpec)
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return "", err
	}

	return tempSpec.Name(), nil
}

func main() {
	exitCode := 0

	yumExtras := GetInputAsArray("yum-extras")
	if len(yumExtras) > 0 {
		if err := installExtraPackages(yumExtras...); err != nil {
			githubactions.Errorf("Error installing extra packages: %s", err)
			os.Exit(1)
		}
	}

	for _, spec := range os.Args[1:] {
		packageName := strings.TrimSuffix(path.Base(spec), path.Ext(spec))

		githubactions.Infof("Building package %s\n", packageName)

		if err := buildPackage(spec); err != nil {
			githubactions.Errorf("Error building package %s: %s", packageName, err)
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}
